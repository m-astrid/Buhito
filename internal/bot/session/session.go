// Package session — конечный автомат сессии тренировки (in-memory, по chatID).
package session

import (
	"sync"

	trainerv1 "github.com/evgshumakova/mini-owl/gen/trainer/v1"
)

// State — состояние FSM пользователя.
type State int

const (
	StateIdle State = iota
	StateSelectLanguage
	StateSelectLevel
	StateSelectTopics
	StateSelectCount
	StateInSession
	StateSummary
)

// TopicChoice — тема на экране выбора (грамматика или лексика).
type TopicChoice struct {
	ID        int64
	Title     string
	IsGrammar bool
	Selected  bool
}

// Session — состояние одного чата. Поля защищены мьютексом (хендлеры
// телебота выполняются конкурентно).
type Session struct {
	mu     sync.Mutex
	ChatID int64
	State  State

	// Выбор параметров.
	LanguageCode string
	LanguageName string
	LevelCode    string
	Topics       []TopicChoice
	WantCount    int

	// Цикл карточек.
	Cards        []*trainerv1.Card
	CurrentIndex int // индекс текущей карточки
	AnswerShown  bool
}

// Lock/Unlock — доступ к полям сессии под мьютексом.
func (s *Session) Lock()   { s.mu.Lock() }
func (s *Session) Unlock() { s.mu.Unlock() }

// SelectedGrammarIDs возвращает выбранные грамматические темы.
func (s *Session) SelectedGrammarIDs() []int64 {
	return s.selectedIDs(true)
}

// SelectedLexicaIDs возвращает выбранные лексические темы.
func (s *Session) SelectedLexicaIDs() []int64 {
	return s.selectedIDs(false)
}

func (s *Session) selectedIDs(grammar bool) []int64 {
	var ids []int64
	for _, t := range s.Topics {
		if t.Selected && t.IsGrammar == grammar {
			ids = append(ids, t.ID)
		}
	}
	return ids
}

// SelectedCount — сколько тем выбрано.
func (s *Session) SelectedCount() int {
	n := 0
	for _, t := range s.Topics {
		if t.Selected {
			n++
		}
	}
	return n
}

// CurrentCard возвращает текущую карточку цикла (nil, если цикл закончен).
func (s *Session) CurrentCard() *trainerv1.Card {
	if s.CurrentIndex < 0 || s.CurrentIndex >= len(s.Cards) {
		return nil
	}
	return s.Cards[s.CurrentIndex]
}

// Advance переходит к следующей карточке; возвращает false, если карточек больше нет.
func (s *Session) Advance() bool {
	if s.CurrentIndex+1 >= len(s.Cards) {
		return false
	}
	s.CurrentIndex++
	s.AnswerShown = false
	return true
}

// TotalCards — сколько карточек в текущей сессии.
func (s *Session) TotalCards() int { return len(s.Cards) }

// Reset очищает сессию до выбора языка.
func (s *Session) Reset() {
	*s = Session{ChatID: s.ChatID, State: StateSelectLanguage}
}

// Store — потокобезопасное хранилище сессий по chatID (in-memory, MVP).
type Store struct {
	mu       sync.Mutex
	sessions map[int64]*Session
}

func NewStore() *Store {
	return &Store{sessions: make(map[int64]*Session)}
}

// Get возвращает существующую сессию или nil.
func (st *Store) Get(chatID int64) *Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.sessions[chatID]
}

// GetOrCreate возвращает сессию, создавая новую (Idle) при отсутствии.
func (st *Store) GetOrCreate(chatID int64) *Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.sessions[chatID]
	if !ok {
		s = &Session{ChatID: chatID, State: StateIdle}
		st.sessions[chatID] = s
	}
	return s
}

// Delete удаляет сессию.
func (st *Store) Delete(chatID int64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.sessions, chatID)
}
