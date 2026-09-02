package localconfig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Store struct {
	path string
}

func NewStore(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("configuration path is required")
	}
	return &Store{path: path}, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Load() (Overrides, bool, error) {
	return Load(s.path)
}

func (s *Store) Set(ctx context.Context, key Key, value string) error {
	overrides, err := ParseOverride(key, value)
	if err != nil {
		return err
	}
	canonical, ok := OverrideValue(overrides, key)
	if !ok {
		return fmt.Errorf("configuration key %q has no value", key)
	}

	return s.withLock(ctx, func() error {
		root, err := s.loadDocument()
		if err != nil {
			return err
		}
		setDocumentValue(root, key, canonical)
		return s.writeDocument(root)
	})
}

func (s *Store) Unset(ctx context.Context, key Key) (bool, error) {
	if _, err := ParseKey(string(key)); err != nil {
		return false, err
	}
	removed := false
	err := s.withLock(ctx, func() error {
		root, found, err := s.loadExistingDocument()
		if err != nil || !found {
			return err
		}
		removed = unsetDocumentValue(root, key)
		if !removed {
			return nil
		}
		return s.writeDocument(root)
	})
	return removed, err
}

func (s *Store) withLock(ctx context.Context, action func() error) error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create configuration directory %q: %w; check directory permissions", directory, err)
	}
	lock, err := acquireConfigLock(ctx, s.path+".lock")
	if err != nil {
		return fmt.Errorf("lock configuration %q: %w; check file and directory permissions", s.path, err)
	}
	defer lock.release()
	return action()
}

func (s *Store) loadDocument() (*yaml.Node, error) {
	root, found, err := s.loadExistingDocument()
	if err != nil {
		return nil, err
	}
	if found {
		return root, nil
	}
	return newDocument(), nil
}

func (s *Store) loadExistingDocument() (*yaml.Node, bool, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read configuration %q: %w; check file permissions", s.path, err)
	}
	if _, err := Parse(data); err != nil {
		return nil, true, fmt.Errorf("invalid configuration %q: %w; fix it or move it aside before retrying", s.path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, true, fmt.Errorf("decode configuration %q: %w", s.path, err)
	}
	return &root, true, nil
}

func newDocument() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Tag: "!!str", Value: "version"},
				{Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"},
			},
		}},
	}
}

func setDocumentValue(root *yaml.Node, key Key, value string) {
	sectionName, fieldName := splitKey(key)
	rootMapping := root.Content[0]
	section := mappingValue(rootMapping, sectionName)
	if section == nil {
		section = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		rootMapping.Content = append(rootMapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: sectionName}, section)
	} else if section.Kind != yaml.MappingNode {
		section.Kind = yaml.MappingNode
		section.Tag = "!!map"
		section.Value = ""
		section.Content = nil
	}
	valueNode := mappingValue(section, fieldName)
	if valueNode == nil {
		valueNode = &yaml.Node{}
		section.Content = append(section.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fieldName}, valueNode)
	}
	valueNode.Kind = yaml.ScalarNode
	valueNode.Value = value
	valueNode.Style = 0
	switch definitionType(key) {
	case "boolean":
		valueNode.Tag = "!!bool"
	case "integer":
		valueNode.Tag = "!!int"
	default:
		valueNode.Tag = "!!str"
	}
}

func unsetDocumentValue(root *yaml.Node, key Key) bool {
	sectionName, fieldName := splitKey(key)
	section := mappingValue(root.Content[0], sectionName)
	if section == nil {
		return false
	}
	for index := 0; index < len(section.Content); index += 2 {
		if section.Content[index].Value == fieldName {
			section.Content = append(section.Content[:index], section.Content[index+2:]...)
			return true
		}
	}
	return false
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func splitKey(key Key) (string, string) {
	parts := strings.SplitN(string(key), ".", 2)
	return parts[0], parts[1]
}

func definitionType(key Key) string {
	for _, definition := range definitions {
		if definition.Key == key {
			return definition.Type
		}
	}
	return "string"
}

func (s *Store) writeDocument(root *yaml.Node) error {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("finish configuration encoding: %w", err)
	}
	if _, err := Parse(buffer.Bytes()); err != nil {
		return fmt.Errorf("validate updated configuration: %w", err)
	}
	if err := writeConfigAtomically(s.path, buffer.Bytes()); err != nil {
		return fmt.Errorf("write configuration %q: %w; check file and directory permissions", s.path, err)
	}
	return nil
}

func writeConfigAtomically(destination string, data []byte) error {
	return writeConfigAtomicallyWithReplace(destination, data, replaceConfigFile)
}

func writeConfigAtomicallyWithReplace(destination string, data []byte, replace func(string, string) error) error {
	directory := filepath.Dir(destination)
	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	closed = true
	if err := replace(temporaryPath, destination); err != nil {
		return err
	}
	return syncConfigDirectory(directory)
}
