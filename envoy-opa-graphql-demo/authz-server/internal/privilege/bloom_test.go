package privilege

import (
	"testing"
)

func TestEncode_AdminRoles(t *testing.T) {
	t.Parallel()
	encoded, err := Encode([]string{"admin"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if encoded == "" {
		t.Fatal("expected non-empty encoded string")
	}

	// admin 应拥有 read:salary
	has, err := HasPrivilege(encoded, "read:salary")
	if err != nil {
		t.Fatalf("HasPrivilege: %v", err)
	}
	if !has {
		t.Error("expected admin to have read:salary")
	}

	// admin 应拥有 manage:users
	has, err = HasPrivilege(encoded, "manage:users")
	if err != nil {
		t.Fatalf("HasPrivilege: %v", err)
	}
	if !has {
		t.Error("expected admin to have manage:users")
	}
}

func TestEncode_UserRoles(t *testing.T) {
	t.Parallel()
	encoded, err := Encode([]string{"user"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// user 应拥有 read:employee
	has, err := HasPrivilege(encoded, "read:employee")
	if err != nil {
		t.Fatalf("HasPrivilege: %v", err)
	}
	if !has {
		t.Error("expected user to have read:employee")
	}

	// user 不应拥有 read:salary
	has, err = HasPrivilege(encoded, "read:salary")
	if err != nil {
		t.Fatalf("HasPrivilege: %v", err)
	}
	if has {
		t.Error("expected user NOT to have read:salary")
	}
}

func TestEncode_MultipleRoles(t *testing.T) {
	t.Parallel()
	encoded, err := Encode([]string{"user", "hr"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// hr 角色带来 read:salary
	has, err := HasPrivilege(encoded, "read:salary")
	if err != nil {
		t.Fatalf("HasPrivilege: %v", err)
	}
	if !has {
		t.Error("expected user+hr to have read:salary")
	}

	// user 角色带来 read:department
	has, err = HasPrivilege(encoded, "read:department")
	if err != nil {
		t.Fatalf("HasPrivilege: %v", err)
	}
	if !has {
		t.Error("expected user+hr to have read:department")
	}
}

func TestEncode_EmptyRoles(t *testing.T) {
	t.Parallel()
	encoded, err := Encode([]string{})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if encoded == "" {
		t.Fatal("expected non-empty encoded string even for empty roles")
	}

	// 空角色不应拥有任何权限
	has, err := HasPrivilege(encoded, "read:employee")
	if err != nil {
		t.Fatalf("HasPrivilege: %v", err)
	}
	if has {
		t.Error("expected empty roles NOT to have read:employee")
	}
}

func TestEncode_UnknownRole(t *testing.T) {
	t.Parallel()
	encoded, err := Encode([]string{"unknown_role"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	has, err := HasPrivilege(encoded, "read:employee")
	if err != nil {
		t.Fatalf("HasPrivilege: %v", err)
	}
	if has {
		t.Error("expected unknown role NOT to have read:employee")
	}
}

func TestHasPrivilege_InvalidBase64(t *testing.T) {
	t.Parallel()
	_, err := HasPrivilege("not-valid-base64!!!", "read:employee")
	if err == nil {
		t.Error("expected error for invalid base64 input")
	}
}

func TestHasPrivilege_InvalidBloomData(t *testing.T) {
	t.Parallel()
	// Valid base64 but not a valid bloom filter
	_, err := HasPrivilege("aGVsbG8=", "read:employee")
	if err == nil {
		t.Error("expected error for invalid bloom filter data")
	}
}
