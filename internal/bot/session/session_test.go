package session_test

import (
	"sync"
	"testing"

	trainerv1 "github.com/evgshumakova/mini-owl/gen/trainer/v1"
	"github.com/evgshumakova/mini-owl/internal/bot/session"
)

func TestStoreIsolationByChat(t *testing.T) {
	st := session.NewStore()

	a := st.GetOrCreate(1)
	b := st.GetOrCreate(2)
	if a == b {
		t.Fatal("сессии разных чатов не должны совпадать")
	}

	a.State = session.StateSelectTopics
	if st.Get(2).State != session.StateIdle {
		t.Fatalf("состояние утекло между чатами: %d", st.Get(2).State)
	}

	st.Delete(1)
	if got := st.Get(1); got != nil {
		t.Fatal("после Delete сессии быть не должно")
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	st := session.NewStore()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := st.GetOrCreate(int64(i % 4))
			s.Lock()
			s.State = session.StateSelectCount
			s.Unlock()
			st.Delete(int64(i % 5))
		}(i)
	}
	wg.Wait()
}

func TestSelectedIDs(t *testing.T) {
	s := &session.Session{State: session.StateSelectTopics}
	s.Topics = []session.TopicChoice{
		{ID: 1, IsGrammar: true, Selected: true},
		{ID: 2, IsGrammar: true, Selected: false},
		{ID: 3, IsGrammar: false, Selected: true},
		{ID: 4, IsGrammar: false, Selected: false},
	}
	if g := s.SelectedGrammarIDs(); len(g) != 1 || g[0] != 1 {
		t.Fatalf("grammar: %+v", g)
	}
	if l := s.SelectedLexicaIDs(); len(l) != 1 || l[0] != 3 {
		t.Fatalf("lexica: %+v", l)
	}
	if s.SelectedCount() != 2 {
		t.Fatalf("selected count: %d", s.SelectedCount())
	}
}

func cards(n int) []*trainerv1.Card {
	out := make([]*trainerv1.Card, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, &trainerv1.Card{Id: int64(i + 1)})
	}
	return out
}

func TestCardCycle(t *testing.T) {
	s := &session.Session{State: session.StateInSession}
	if s.CurrentCard() != nil {
		t.Fatal("без карточек CurrentCard должен быть nil")
	}

	s.Cards = cards(3)
	s.CurrentIndex = 0
	if s.CurrentCard().GetId() != 1 {
		t.Fatalf("ожидалась карточка 1, получена %d", s.CurrentCard().GetId())
	}
	if !s.Advance() || s.CurrentCard().GetId() != 2 {
		t.Fatal("Advance не перешёл на карточку 2")
	}
	if !s.Advance() || s.CurrentCard().GetId() != 3 {
		t.Fatal("Advance не перешёл на карточку 3")
	}
	if s.Advance() {
		t.Fatal("Advance за пределами списка должен вернуть false")
	}
	if s.CurrentCard().GetId() != 3 {
		t.Fatal("после последнего Advance индекс не должен меняться")
	}
}

func TestReset(t *testing.T) {
	s := &session.Session{
		ChatID:       42,
		State:        session.StateInSession,
		LanguageCode: "it",
		LevelCode:    "B1",
		Topics:       []session.TopicChoice{{ID: 1}},
		Cards:        cards(2),
		CurrentIndex: 1,
	}
	s.Reset()
	if s.State != session.StateSelectLanguage {
		t.Fatalf("после Reset state = %d", s.State)
	}
	if s.LanguageCode != "" || s.LevelCode != "" || s.Topics != nil || s.Cards != nil {
		t.Fatalf("после Reset данные не очищены: %+v", s)
	}
	if s.ChatID != 42 {
		t.Fatalf("ChatID потерян: %d", s.ChatID)
	}
}
