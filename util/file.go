package util

import (
	"bufio"
	"encoding/json"
	"os"
)

func SaveJSON(filename string, v interface{}) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return err
	}
	return nil
}

func ReadLines(filepath string) ([]string, error) {
	file, err := os.Create(filepath)
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
	file, err := os.Create(filepath)
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
