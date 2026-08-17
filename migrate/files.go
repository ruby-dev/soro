package migrate

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	upMarker   = "-- +soro Up"
	downMarker = "-- +soro Down"
)

// LoadDir loads convention-based .sql migrations in lexical filename order.
func LoadDir(directory string) ([]Migration, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return []Migration{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read migrations %s: %w", directory, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		migration, err := LoadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, migration)
	}
	if err := validateMigrations(migrations); err != nil {
		return nil, err
	}
	return migrations, nil
}

// LoadFile parses one migration with explicit Soro Up and Down sections.
func LoadFile(path string) (Migration, error) {
	file, err := os.Open(path)
	if err != nil {
		return Migration{}, fmt.Errorf("open migration %s: %w", path, err)
	}
	defer file.Close()

	var up, down strings.Builder
	section := ""
	seenUp, seenDown := false, false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		switch strings.TrimSpace(line) {
		case upMarker:
			if seenUp || seenDown {
				return Migration{}, fmt.Errorf("migration %s has misplaced or duplicate Up marker", path)
			}
			seenUp, section = true, "up"
			continue
		case downMarker:
			if !seenUp || seenDown {
				return Migration{}, fmt.Errorf("migration %s has misplaced or duplicate Down marker", path)
			}
			seenDown, section = true, "down"
			continue
		}
		switch section {
		case "up":
			up.WriteString(line)
			up.WriteByte('\n')
		case "down":
			down.WriteString(line)
			down.WriteByte('\n')
		default:
			if strings.TrimSpace(line) != "" {
				return Migration{}, fmt.Errorf("migration %s has content before Up marker", path)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Migration{}, fmt.Errorf("read migration %s: %w", path, err)
	}
	if !seenUp || !seenDown {
		return Migration{}, fmt.Errorf("migration %s must contain %q and %q", path, upMarker, downMarker)
	}
	upStatements, err := splitStatements(up.String())
	if err != nil {
		return Migration{}, fmt.Errorf("migration %s Up: %w", path, err)
	}
	downStatements, err := splitStatements(down.String())
	if err != nil {
		return Migration{}, fmt.Errorf("migration %s Down: %w", path, err)
	}
	if len(upStatements) == 0 || len(downStatements) == 0 {
		return Migration{}, fmt.Errorf("migration %s Up and Down sections must not be empty", path)
	}
	return Migration{Name: strings.TrimSuffix(filepath.Base(path), ".sql"), Up: upStatements, Down: downStatements}, nil
}

// splitStatements understands PostgreSQL strings, quoted identifiers, dollar
// quotes, and comments. It deliberately rejects unterminated constructs.
func splitStatements(source string) ([]string, error) {
	var statements []string
	start := 0
	quote := byte(0)
	dollar := ""
	lineComment, blockComment := false, false
	for index := 0; index < len(source); index++ {
		character := source[index]
		if lineComment {
			if character == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if character == '*' && index+1 < len(source) && source[index+1] == '/' {
				blockComment = false
				index++
			}
			continue
		}
		if dollar != "" {
			if strings.HasPrefix(source[index:], dollar) {
				index += len(dollar) - 1
				dollar = ""
			}
			continue
		}
		if quote != 0 {
			if character == quote {
				if index+1 < len(source) && source[index+1] == quote {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		if character == '-' && index+1 < len(source) && source[index+1] == '-' {
			lineComment = true
			index++
			continue
		}
		if character == '/' && index+1 < len(source) && source[index+1] == '*' {
			blockComment = true
			index++
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if character == '$' {
			if end := strings.IndexByte(source[index+1:], '$'); end >= 0 {
				tag := source[index : index+end+2]
				valid := true
				for _, character := range tag[1 : len(tag)-1] {
					if character != '_' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
						valid = false
						break
					}
				}
				if valid {
					dollar = tag
					index += len(tag) - 1
					continue
				}
			}
		}
		if character == ';' {
			statement := strings.TrimSpace(source[start:index])
			if statement != "" {
				statements = append(statements, statement)
			}
			start = index + 1
		}
	}
	if quote != 0 || dollar != "" || blockComment {
		return nil, fmt.Errorf("unterminated SQL quote or comment")
	}
	if statement := strings.TrimSpace(source[start:]); statement != "" {
		return nil, fmt.Errorf("SQL statement must end with a semicolon")
	}
	return statements, nil
}
