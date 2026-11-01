package accounts

import "testing"

func TestSameIDNormalizesUUID(t *testing.T) {
	dashed := "9775d829-cdb8-445e-956a-0f13847a330f"
	plain := "9775D829CDB8445E956A0F13847A330F"
	if !sameID(dashed, plain) {
		t.Fatalf("expected %s to match %s", dashed, plain)
	}
	if sameID(dashed, "f32a7aeb-db22-46b9-a6ad-fbe14c8ab4a6") {
		t.Fatal("different UUIDs should not match")
	}
	if sameID("", dashed) {
		t.Fatal("empty id should not match")
	}
}

func TestRankMetaAndPixelAccount(t *testing.T) {
	label, color := RankMeta("usuario")
	if label != "Usuario" || color != "#059669" {
		t.Fatalf("usuario meta %s %s", label, color)
	}
	label, color = RankMeta("pixel_plus")
	if label != "Pixel+" || color != "#c084fc" {
		t.Fatalf("plus meta %s %s", label, color)
	}
	label, color = RankMeta("fundador")
	if label != "Fundador" || color != "#ef4444" {
		t.Fatalf("fundador meta %s %s", label, color)
	}
	ms := Credentials{Kind: "premium", AccessToken: "ms-token"}
	if !ms.PixelAccount() || ms.PixelPasswordAccount() {
		t.Fatal("premium microsoft should be pixel but not password")
	}
	cracked := Credentials{Kind: "cracked", PixelplayCredentials: PixelplayCredentials{Production: &Token{Value: "tok"}}}
	if !cracked.PixelAccount() || !cracked.PixelPasswordAccount() {
		t.Fatal("cracked pixel should use password path")
	}
	local := Credentials{Username: "pepito"}
	if local.PixelAccount() {
		t.Fatal("offline local is not pixel")
	}
}

func TestMatchAccountByUsername(t *testing.T) {
	a := Credentials{ID: "9775d829-cdb8-445e-956a-0f13847a330f", Username: "Joacoorandom"}
	if !matchAccount(a, "joacoorandom") {
		t.Fatal("expected username match")
	}
	if matchAccount(a, "f32a7aeb-db22-46b9-a6ad-fbe14c8ab4a6") {
		t.Fatal("unknown uuid should not match")
	}
}
