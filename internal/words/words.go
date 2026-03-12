package words

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math/rand"
)

//go:embed uk.json
var ukWords []byte

//go:embed en.json
var enWords []byte

//go:embed ru.json
var ruWords []byte

type difficultyDict struct {
	Easy   []string `json:"easy"`
	Medium []string `json:"medium"`
	Hard   []string `json:"hard"`
}

type Service struct {
	dictionaries map[string]difficultyDict
}

func NewService() (*Service, error) {
	s := &Service{dictionaries: make(map[string]difficultyDict)}
	for lang, data := range map[string][]byte{"uk": ukWords, "en": enWords, "ru": ruWords} {
		var d difficultyDict
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("failed to load %s dictionary: %w", lang, err)
		}
		s.dictionaries[lang] = d
	}
	return s, nil
}

// GetWords returns n random words for the given language and difficulty, avoiding already used words.
// If the difficulty list is empty, falls back to easy.
func (s *Service) GetWords(lang, difficulty string, n int, used []string) ([]string, error) {
	d, ok := s.dictionaries[lang]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", lang)
	}

	var pool []string
	switch difficulty {
	case "medium":
		pool = d.Medium
	case "hard":
		pool = d.Hard
	default:
		pool = d.Easy
	}

	// Fallback to easy if selected difficulty list is empty
	if len(pool) == 0 {
		pool = d.Easy
	}

	usedSet := make(map[string]bool, len(used))
	for _, w := range used {
		usedSet[w] = true
	}
	available := make([]string, 0, len(pool))
	for _, w := range pool {
		if !usedSet[w] {
			available = append(available, w)
		}
	}
	if len(available) < n {
		n = len(available)
	}
	rand.Shuffle(len(available), func(i, j int) { available[i], available[j] = available[j], available[i] })
	return available[:n], nil
}
