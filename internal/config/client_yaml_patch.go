package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"haovpn/internal/fileutil"
)

// clientSavePatch GUI 写回 client.yaml 时会更改的字段（其余段与注释保留不动）。
type clientSavePatch struct {
	Address          string
	Username         string
	RememberPassword bool
	Password         string
	LocalLANs        []string // nil=不改；非 nil（含空切片）=覆盖 local_lans
}

// SaveClient 将 GUI 修改过的偏好写回 client.yaml（权限 0600）。
func SaveClient(path string, cfg *ClientConfig) error {
	if cfg == nil {
		return fmt.Errorf("配置不能为空")
	}
	out := *cfg
	out.ApplyDefaults()
	patch := clientSavePatch{
		Address:          out.Server.Address,
		Username:         out.Auth.Username,
		RememberPassword: out.Auth.RememberPassword,
		LocalLANs:        append([]string{}, out.LocalLANs...),
	}
	if out.Auth.RememberPassword {
		patch.Password = out.Auth.Password
	}
	return patchClientYAML(path, patch)
}

func patchClientYAML(path string, patch clientSavePatch) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if _, err := ensureFile(path, clientYAMLTemplate); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("读取 %s: %w", path, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("解析 %s: %w", path, err)
	}
	root := yamlRootMapping(&doc)
	if root == nil {
		return fmt.Errorf("配置 %s 根节点须为 mapping", path)
	}

	if err := setNestedScalar(root, []string{"server", "address"}, patch.Address); err != nil {
		return err
	}
	authMap, err := ensureNestedMapping(root, "auth")
	if err != nil {
		return err
	}
	setMappingScalar(authMap, "username", patch.Username)
	setMappingBool(authMap, "remember_password", patch.RememberPassword)
	if patch.RememberPassword && patch.Password != "" {
		setMappingScalar(authMap, "password", patch.Password)
	} else {
		deleteMappingKey(authMap, "password")
	}
	setMappingStringSequence(root, "local_lans", patch.LocalLANs)
	deleteMappingKey(root, "peer")

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("序列化 client 配置: %w", err)
	}
	if err := fileutil.WriteFileAtomic(path, out, 0o600); err != nil {
		return fmt.Errorf("写入 %s: %w", path, err)
	}
	return nil
}
