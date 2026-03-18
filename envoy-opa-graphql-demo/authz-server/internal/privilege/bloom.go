package privilege

import (
	"encoding/base64"

	"github.com/bits-and-blooms/bloom/v3"
)

const (
	estimatedMaxPrivileges = 1024 // Bloom Filter 预估容量
	falsePositiveRate      = 0.01 // 误判率
)

// RolePrivileges Demo 硬编码：角色 → 权限列表映射。
var RolePrivileges = map[string][]string{
	"admin": {
		"read:employee", "write:employee", "delete:employee",
		"read:salary", "write:salary",
		"read:department", "write:department",
		"manage:users", "manage:roles",
	},
	"user": {
		"read:employee", "read:department",
	},
	"hr": {
		"read:employee", "write:employee",
		"read:salary", "write:salary",
		"read:department",
	},
}

// Encode 根据角色列表收集所有权限，构建 Bloom Filter，返回 base64 编码的字符串。
func Encode(roles []string) (string, error) {
	filter := bloom.NewWithEstimates(estimatedMaxPrivileges, falsePositiveRate)
	for _, role := range roles {
		if privs, ok := RolePrivileges[role]; ok {
			for _, p := range privs {
				filter.AddString(p)
			}
		}
	}
	data, err := filter.MarshalBinary()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// HasPrivilege 解码 base64 的 Bloom Filter 字符串，检查是否包含指定权限。
func HasPrivilege(encoded string, privilege string) (bool, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false, err
	}
	filter := &bloom.BloomFilter{}
	if err := filter.UnmarshalBinary(data); err != nil {
		return false, err
	}
	return filter.TestString(privilege), nil
}
