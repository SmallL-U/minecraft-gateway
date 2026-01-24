package util

import (
	"bufio"
	"os"

	"github.com/goccy/go-yaml"
)

func SaveYAML(filepath string, v interface{}) error {
	data, err := yaml.MarshalWithOptions(v, yaml.IndentSequence(true))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, data, 0644)
}

func ReadLines(filepath string) ([]string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func SaveLines(filepath string, lines []string) error {
	file, err := os.OpenFile(filepath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	for _, line := range lines {
		if _, err := file.WriteString(line + LineSeparator()); err != nil {
			return err
		}
	}
	return nil
}
