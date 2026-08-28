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
	Password         string // remember_password=false 时删除 auth.password 键
}

// SaveClient 将 GUI 修改过的偏好写回 client.yaml（权限 0600）。
//
// 使用 yaml.Node 局部 patch，保留未改段的中文注释；并删除 legacy peer 段。
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
	}
	if out.Auth.RememberPassword {
		patch.Password = out.Auth.Password
	}
	return patchClientYAML(path, patch)
}

// patchClientYAML 读盘 → 局部更新 auth/server → 删 peer → 写盘。
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
	deleteMappingKey(root, "peer")

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("序列化 client 配置: %w", err)
	}
	// 原子写：避免 GUI 写盘中途崩溃留下半截 YAML。
	if err := fileutil.WriteFileAtomic(path, out, 0o600); err != nil {
		return fmt.Errorf("写入 %s: %w", path, err)
	}
	return nil
}

func yamlRootMapping(doc *yaml.Node) *yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	return root
}

func setNestedScalar(root *yaml.Node, path []string, value string) error {
	if len(path) == 0 {
		return fmt.Errorf("patch 路径为空")
	}
	if len(path) == 1 {
		setMappingScalar(root, path[0], value)
		return nil
	}
	child, err := ensureNestedMapping(root, path[0])
	if err != nil {
		return err
	}
	return setNestedScalar(child, path[1:], value)
}

func ensureNestedMapping(root *yaml.Node, key string) (*yaml.Node, error) {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("父节点须为 mapping")
	}
	_, val := mappingEntry(root, key)
	if val != nil {
		if val.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("键 %q 须为 mapping", key)
		}
		return val, nil
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	root.Content = append(root.Content, keyNode, valNode)
	return valNode, nil
}

func mappingEntry(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

func setMappingScalar(m *yaml.Node, key, value string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	if _, val := mappingEntry(m, key); val != nil {
		val.Kind = yaml.ScalarNode
		val.Tag = "!!str"
		val.Value = value
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func setMappingBool(m *yaml.Node, key string, value bool) {
	s := "false"
	if value {
		s = "true"
	}
	if _, val := mappingEntry(m, key); val != nil {
		val.Kind = yaml.ScalarNode
		val.Tag = "!!bool"
		val.Value = s
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: s},
	)
}

func deleteMappingKey(m *yaml.Node, key string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}
