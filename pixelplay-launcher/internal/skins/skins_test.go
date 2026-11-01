package skins

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestSessionLikeProfileHasTextures(t *testing.T) {
	raw := sessionLikeProfile("9775d829cdb8445e956a0f13847a330f", "Joaco", "https://api.pixelplay.gg/api/v1/player/9775d829cdb8445e956a0f13847a330f/skin.png", "https://api.pixelplay.gg/api/v1/player/9775d829cdb8445e956a0f13847a330f/cape.png", "slim")
	var prof struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Properties []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &prof); err != nil {
		t.Fatal(err)
	}
	if prof.ID != "9775d829cdb8445e956a0f13847a330f" || prof.Name != "Joaco" {
		t.Fatalf("profile %s %s", prof.ID, prof.Name)
	}
	if len(prof.Properties) == 0 || prof.Properties[0].Name != "textures" {
		t.Fatal("missing textures property")
	}
	decoded, err := base64.StdEncoding.DecodeString(prof.Properties[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	var tex struct {
		Textures struct {
			Skin struct {
				URL      string `json:"url"`
				Metadata struct {
					Model string `json:"model"`
				} `json:"metadata"`
			} `json:"SKIN"`
			Cape struct {
				URL string `json:"url"`
			} `json:"CAPE"`
		} `json:"textures"`
	}
	if json.Unmarshal(decoded, &tex) != nil {
		t.Fatal(string(decoded))
	}
	if tex.Textures.Skin.URL == "" || tex.Textures.Cape.URL == "" || tex.Textures.Skin.Metadata.Model != "slim" {
		t.Fatalf("textures %+v", tex.Textures)
	}
}

func TestPixelOrLocalProfileNeverErrors(t *testing.T) {
	raw := pixelOrLocalProfile("not-a-uuid", "")
	if len(raw) < 10 {
		t.Fatal("expected synthetic profile")
	}
	var prof map[string]any
	if json.Unmarshal(raw, &prof) != nil {
		t.Fatal(string(raw))
	}
	if prof["name"] != "Player" {
		t.Fatalf("name %v", prof["name"])
	}
}
