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

func TestMatchAccountByUsername(t *testing.T) {
	a := Credentials{ID: "9775d829-cdb8-445e-956a-0f13847a330f", Username: "Joacoorandom"}
	if !matchAccount(a, "joacoorandom") {
		t.Fatal("expected username match")
	}
	if matchAccount(a, "f32a7aeb-db22-46b9-a6ad-fbe14c8ab4a6") {
		t.Fatal("unknown uuid should not match")
	}
}
