import sys,json,threading,base64,os,glob,subprocess,fcntl,struct,math,time
import gi
gi.require_version('Gst','1.0');gi.require_version('GstWebRTC','1.0');gi.require_version('GstSdp','1.0')
from gi.repository import Gst,GstWebRTC,GstSdp,GLib
Gst.init(None)
lock=threading.Lock()
def emit(o):
 with lock: print(json.dumps(o,separators=(',',':')),flush=True)
def devices():
 out=[]
 try:
  for s in json.loads(subprocess.check_output(['pactl','-f','json','list','sources'])):
   if not s['name'].endswith('.monitor'):out.append(dict(kind='audioinput',deviceId=s['name'],label=s.get('description',s['name'])))
  for s in json.loads(subprocess.check_output(['pactl','-f','json','list','sinks'])):out.append(dict(kind='audiooutput',deviceId=s['name'],label=s.get('description',s['name'])))
 except Exception:pass
 for path in sorted(glob.glob('/dev/video*')):
  try:
   with open(path,'rb',buffering=0) as fd:
    buf=bytearray(104);fcntl.ioctl(fd,0x80685600,buf,True)
    caps=struct.unpack_from('I',buf,88)[0] or struct.unpack_from('I',buf,84)[0]
    if not caps&1:continue
    label=bytes(buf[16:48]).split(b'\0')[0].decode(errors='replace')
   out.append(dict(kind='videoinput',deviceId=path,label=label))
  except Exception:pass
 return out
if '--devices' in sys.argv:emit(devices());sys.exit(0)
class Engine:
 def __init__(self):
  self.pipeline=None;self.pc=None;self.id='';self.camera=None;self.video_wanted=False;self.ice=[];self.remote=False;self.polite=False;self.negotiated=False;self.making=False;self.video_frames=0;self.audio_in=0;self.audio_out=0;self.mids=[];self.last_remote=0;self.retry=0
 def start(self,o):
  if self.id==o['call'] and self.pipeline:return
  self.stop();self.id=o['call'];self.polite=o.get('direction')=='in'
  all_devices=devices();emit({'event':'devices','devices':all_devices})
  mic=o.get('mic','')
  if not mic or mic=='default' or not any(d['deviceId']==mic for d in all_devices):
   physical=[d for d in all_devices if d['kind']=='audioinput' and ('alsa_input' in d['deviceId'])]
   mic=next((d['deviceId'] for d in physical if 'quadcast' in d['label'].lower()),'') or next((d['deviceId'] for d in physical if 'mic' in d['label'].lower()),'') or (physical[0]['deviceId'] if physical else '')
  self.pipeline=Gst.Pipeline.new('pixel-call');self.pc=Gst.ElementFactory.make('webrtcbin','rtc')
  if not self.pc:raise RuntimeError('Instala gstreamer1.0-plugins-bad y gstreamer1.0-nice')
  self.pc.set_property('bundle-policy',GstWebRTC.WebRTCBundlePolicy.MAX_BUNDLE)
  self.pc.set_property('stun-server','stun://stun.l.google.com:19302');self.pipeline.add(self.pc)
  self.pc.connect('on-ice-candidate',self.candidate)
  self.pc.connect('on-negotiation-needed',lambda _:GLib.idle_add(self.offer))
  self.pc.connect('pad-added',self.incoming)
  self.pc.connect('notify::connection-state',self.connection)
  self.audio=Gst.parse_bin_from_description('pulsesrc name=mic ! audioconvert ! audioresample ! audio/x-raw,rate=48000,channels=1 ! volume name=mute ! level name=level interval=100000000 ! opusenc bitrate=32000 audio-type=voice ! rtpopuspay name=audio_pay pt=111 ! capsfilter name=audio_caps caps="application/x-rtp,media=audio,encoding-name=OPUS,payload=111"',True)
  self.audio.get_by_name('mic').set_property('device',mic or None)
  self.audio.get_by_name('mute').set_property('mute',o.get('muted',False));self.pipeline.add(self.audio)
  self.audio.get_static_pad('src').add_probe(Gst.PadProbeType.BUFFER,self.count_audio,'out')
  self.audio.get_static_pad('src').link(self.pc.request_pad_simple('sink_%u'))
  # Reserve video before negotiation, but send no video packets until the camera is enabled.
  self.video=Gst.parse_bin_from_description('input-selector name=selector ! videoconvert ! videoscale ! videorate ! video/x-raw,width=480,height=270,framerate=12/1 ! tee name=localtee localtee. ! queue ! vp8enc deadline=1 cpu-used=8 target-bitrate=350000 keyframe-max-dist=24 ! rtpvp8pay name=video_pay pt=96 ! valve name=video_gate drop=true drop-mode=forward-sticky-events ! capsfilter name=video_caps caps="application/x-rtp,media=video,encoding-name=VP8,payload=96" localtee. ! queue leaky=downstream max-size-buffers=1 ! jpegenc quality=65 ! appsink name=local_preview emit-signals=true sync=false max-buffers=1 drop=true videotestsrc is-live=true pattern=black ! video/x-raw,width=480,height=270,framerate=12/1 ! selector.',True)
  self.pipeline.add(self.video);self.video.get_static_pad('src').link(self.pc.request_pad_simple('sink_%u'))
  self.black_pad=self.video.get_by_name('selector').get_property('active-pad')
  self.video.get_by_name('local_preview').connect('new-sample',lambda sink:self.frame(sink,'local'))
  self.output=o.get('speaker','');self.deafened=o.get('deafened',False)
  bus=self.pipeline.get_bus();bus.add_signal_watch();bus.connect('message',self.bus)
  self.audio_in=self.audio_out=self.video_frames=0
  self.pipeline.set_state(Gst.State.READY if self.polite else Gst.State.PLAYING)
  GLib.timeout_add_seconds(1,self.stats,self.id)
  if o.get('video'):GLib.timeout_add(200,lambda:self.set_camera(True,o.get('camera','')))
  emit({'event':'status','status':'Conectando audio/video directo…','mic':mic})
 def count_audio(self,pad,info,side):
  if side=='in':self.audio_in+=1
  else:self.audio_out+=1
  return Gst.PadProbeReturn.OK
 def stats(self,call):
  if not self.pc or self.id!=call:return False
  if self.last_remote and time.monotonic()-self.last_remote>1.5:
   self.last_remote=0;emit({'event':'frame','side':'remote','frame':None})
  emit({'event':'stats','audioIn':self.audio_in,'audioOut':self.audio_out,'videoFrames':self.video_frames});return True
 def candidate(self,pc,line,candidate):
  if not candidate:return
  mid=self.mids[line] if line<len(self.mids) else str(line)
  emit({'event':'signal','action':'ice','call':self.id,'candidate':candidate,'sdpMid':mid,'sdpMLineIndex':line})
 def offer(self,restart=False):
  if not self.pc or self.making or (self.negotiated and not restart) or (self.polite and not self.negotiated and not restart):return False
  state=self.pc.get_property('signaling-state')
  if state!=GstWebRTC.WebRTCSignalingState.STABLE:return False
  self.making=True
  options=Gst.Structure.new_empty('options');options.set_value('ice-restart',bool(restart))
  promise=Gst.Promise.new_with_change_func(lambda p,*_:self.created(p,'offer'),None,None)
  self.pc.emit('create-offer',options,promise);return False
 def created(self,p,kind):
  reply=p.get_reply();desc=reply.get_value(kind) if reply else None
  if desc is None:emit({'event':'error','error':str(reply)});self.making=False;return
  GLib.idle_add(self.local,desc.copy(),kind)
 def local(self,desc,kind):
  if not self.pc:return False
  text=desc.sdp.as_text()
  self.mids=[line[6:] for line in text.splitlines() if line.startswith('a=mid:')]
  self.pc.emit('set-local-description',desc,Gst.Promise.new())
  self.making=False;self.negotiated=True
  emit({'event':'signal','action':kind,'call':self.id,'sdp':text});return False
 def signal(self,o):
  if not self.pc or o.get('call')!=self.id:return
  action=o.get('action')
  if action in ('offer','answer'):
   if action=='offer' and self.pc.get_property('signaling-state')!=GstWebRTC.WebRTCSignalingState.STABLE:
    if not self.polite:return
    emit({'event':'error','error':'Conflicto de negociación, pulsa Reintentar'});return
   _,sdp=GstSdp.SDPMessage.new();GstSdp.sdp_message_parse_buffer(o['sdp'].encode(),sdp)
   if action=='offer':
    for i in range(sdp.medias_len()):
     media=sdp.get_media(i);kind=media.get_media();codec='opus' if kind=='audio' else 'vp8'
     for j in range(media.attributes_len()):
      attr=media.get_attribute(j)
      if attr.key=='rtpmap' and attr.value.split(' ',1)[-1].lower().startswith(codec+'/'):
       pt=int(attr.value.split(' ',1)[0]);bin=self.audio if kind=='audio' else self.video
       bin.get_by_name(kind+'_pay').set_property('pt',pt)
       caps=Gst.Caps.from_string(f'application/x-rtp,media={kind},encoding-name={codec.upper()},payload={pt},clock-rate={48000 if kind=="audio" else 90000}')
       bin.get_by_name(kind+'_caps').set_property('caps',caps)
       transceiver=self.pc.emit('get-transceiver',i)
       if transceiver:transceiver.set_property('codec-preferences',caps)
       break
   desc=GstWebRTC.WebRTCSessionDescription.new(GstWebRTC.WebRTCSDPType.OFFER if action=='offer' else GstWebRTC.WebRTCSDPType.ANSWER,sdp)
   def apply():
    if not self.pc:return False
    promise=Gst.Promise.new_with_change_func(lambda p,*_:self.remote_result(p,action),None,None)
    self.pc.emit('set-remote-description',desc,promise);return False
   if self.polite and not self.negotiated:
    self.pipeline.set_state(Gst.State.PLAYING);GLib.timeout_add(300,apply)
   else:apply()
  elif action=='ice':
   if self.remote:self.pc.emit('add-ice-candidate',o.get('sdpMLineIndex',0),o['candidate'])
   else:self.ice.append(o)
 def remote_result(self,p,action):
  reply=p.get_reply()
  if reply and reply.has_field('error'):
   emit({'event':'error','error':str(reply.get_value('error'))});return
  GLib.idle_add(self.remote_set,action)
 def remote_set(self,action):
  if not self.pc:return False
  self.remote=True;self.negotiated=True
  for o in self.ice:self.pc.emit('add-ice-candidate',o.get('sdpMLineIndex',0),o['candidate'])
  self.ice=[]
  if action=='offer':self.pc.emit('create-answer',None,Gst.Promise.new_with_change_func(lambda p,*_:self.created(p,'answer'),None,None))
  return False
 def connection(self,*_):
  if not self.pc:return
  state=self.pc.get_property('connection-state').value_nick
  if state=='connected':self.retry=0
  elif state in ('disconnected','failed'):GLib.timeout_add_seconds(5,self.reconnect,self.id)
  emit({'event':'status','connection':state,'status':'Conectado · audio/video P2P' if state=='connected' else 'Conexión directa · '+state})
 def reconnect(self,call):
  if not self.pc or call!=self.id or self.pc.get_property('connection-state').value_nick=='connected':return False
  if self.retry>=3:emit({'event':'status','status':'No se pudo conectar · Reintentar conexión'});return False
  self.retry+=1;self.offer(True);GLib.timeout_add_seconds(15,self.reconnect,call);return False
 def incoming(self,_,pad):
  if pad.get_direction()!=Gst.PadDirection.SRC:return
  caps=pad.get_current_caps() or pad.query_caps(None);media=caps.get_structure(0).get_string('media')
  if media=='audio':
   pad.add_probe(Gst.PadProbeType.BUFFER,self.count_audio,'in')
   branch=Gst.parse_bin_from_description('rtpopusdepay ! opusdec ! audioconvert ! audioresample ! volume name=deafen ! pulsesink name=output sync=false',True)
   branch.get_by_name('deafen').set_property('mute',self.deafened)
   if self.output and self.output!='default':branch.get_by_name('output').set_property('device',self.output)
  else:
   branch=Gst.parse_bin_from_description('rtpvp8depay ! vp8dec ! videoconvert ! videoscale ! video/x-raw,width=480,height=270 ! jpegenc quality=65 ! appsink name=remote_preview emit-signals=true sync=false drop=true max-buffers=1',True)
   branch.get_by_name('remote_preview').connect('new-sample',lambda sink:self.frame(sink,'remote'))
  self.pipeline.add(branch);branch.sync_state_with_parent();pad.link(branch.get_static_pad('sink'))
 def frame(self,sink,side):
  sample=sink.emit('pull-sample')
  if sample and (side!='local' or self.video_wanted):
   buf=sample.get_buffer();ok,m=buf.map(Gst.MapFlags.READ)
   if ok:
    emit({'event':'frame','side':side,'frame':'data:image/jpeg;base64,'+base64.b64encode(m.data).decode()});buf.unmap(m)
    if side=='remote':self.video_frames+=1;self.last_remote=time.monotonic()
  return Gst.FlowReturn.OK
 def set_camera(self,on,device=''):
  if not self.pipeline:return False
  if on and self.camera is None:
   device=device if device.startswith('/dev/video') else next((d['deviceId'] for d in devices() if d['kind']=='videoinput'),'')
   if not device:emit({'event':'error','error':'No se encontró una cámara'});return False
   self.camera=Gst.parse_bin_from_description('v4l2src name=cam ! videoconvert ! videoscale ! videorate ! video/x-raw,width=480,height=270,framerate=12/1 ! queue',True)
   self.camera.get_by_name('cam').set_property('device',device);self.video.add(self.camera)
   selector=self.video.get_by_name('selector');self.camera_pad=selector.request_pad_simple('sink_%u');self.camera.get_static_pad('src').link(self.camera_pad)
   self.camera.sync_state_with_parent();selector.set_property('active-pad',self.camera_pad)
  if not on and self.camera:
   self.video.get_by_name('selector').set_property('active-pad',self.black_pad)
   self.camera.set_state(Gst.State.NULL);self.camera.get_static_pad('src').unlink(self.camera_pad)
   self.video.get_by_name('selector').release_request_pad(self.camera_pad);self.video.remove(self.camera);self.camera=None
  self.video_wanted=bool(on);self.video.get_by_name('video_gate').set_property('drop',not on)
  if not on:emit({'event':'frame','side':'local','frame':None})
  return False
 def command(self,o):
  try:
   a=o.get('action')
   if a=='start':self.start(o)
   elif a=='signal':self.signal(o['signal'])
   elif a=='camera':
    if self.camera and o.get('device'):self.set_camera(False)
    self.set_camera(o['on'],o.get('device',''))
   elif a=='mute' and self.pipeline:self.audio.get_by_name('mute').set_property('mute',o['on'])
   elif a=='mic' and self.pipeline:
    mic=self.audio.get_by_name('mic');mic.set_state(Gst.State.NULL)
    mic.set_property('device',o.get('device') or None);mic.sync_state_with_parent()
   elif a=='speaker':
    self.output=o.get('device','');element=self.pipeline.get_by_name('output') if self.pipeline else None
    if element:element.set_property('device',self.output if self.output!='default' else None)
   elif a=='deafen':
    self.deafened=o['on'];element=self.pipeline.get_by_name('deafen') if self.pipeline else None
    if element:element.set_property('mute',self.deafened)
   elif a=='restart':self.offer(True)
   elif a=='stop':self.stop()
  except Exception as e:emit({'event':'error','error':str(e)})
  return False
 def bus(self,bus,message):
  if message.type==Gst.MessageType.ERROR:
   error,debug=message.parse_error();emit({'event':'error','error':str(error),'debug':debug,'source':message.src.get_name()})
  elif message.type==Gst.MessageType.ELEMENT:
   s=message.get_structure()
   if s and s.get_name()=='level':
    rms=s.get_value('rms');emit({'event':'level','level':10**(rms[0]/20) if rms else 0})
 def stop(self):
  if self.pipeline:self.pipeline.set_state(Gst.State.NULL)
  self.pipeline=None;self.pc=None;self.camera=None;self.id='';self.remote=False;self.ice=[];self.negotiated=False;self.making=False;self.last_remote=0;self.video_wanted=False;self.retry=0
engine=Engine();loop=GLib.MainLoop()
def read():
 for line in sys.stdin:
  try:GLib.idle_add(engine.command,json.loads(line))
  except Exception as e:emit({'event':'error','error':str(e)})
 GLib.idle_add(loop.quit)
threading.Thread(target=read,daemon=True).start();loop.run();engine.stop()
