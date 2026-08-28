package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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
