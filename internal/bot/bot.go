// Пакет bot — Telegram-бот: клавиатуры, сценарии, FSM.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	trainerv1 "github.com/evgshumakova/mini-owl/gen/trainer/v1"
	"github.com/evgshumakova/mini-owl/internal/bot/session"
)

// Bot — Telegram-бот-тренажёр.
type Bot struct {
	tb    *tele.Bot
	store *session.Store
	api   *TrainerClient
	log   *slog.Logger
}

// NewBot создаёт бота с long polling и регистрирует хендлеры.
func NewBot(token, grpcAddr string, log *slog.Logger) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}
	tb, err := tele.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("telebot: %w", err)
	}
	api, err := NewTrainerClient(grpcAddr, 5*time.Second)
	if err != nil {
		return nil, err
	}

	b := &Bot{tb: tb, store: session.NewStore(), api: api, log: log}
	b.register()
	return b, nil
}

// Start запускает long polling (блокирует до остановки).
func (b *Bot) Start() { b.tb.Start() }

// Stop останавливает бота.
func (b *Bot) Stop() { b.tb.Stop() }

func (b *Bot) register() {
	b.tb.Handle("/start", b.onStart)
	b.tb.Handle("/train", b.onTrain)
	b.tb.Handle("/stop", b.onStop)
	b.tb.Handle("/help", b.onHelp)
	b.tb.Handle(tele.OnCallback, b.onCallback)
	b.tb.Handle(tele.OnVoice, b.onVoice)
	b.tb.Handle(tele.OnText, b.onText)
}

// --- Команды ---

func (b *Bot) onStart(c tele.Context) error {
	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	defer s.Unlock()

	s.State = session.StateIdle
	return c.Send(
		"Привет! Это тренажёр перевода: я даю фразу на русском, "+
			"ты произносишь перевод вслух и записываешь голосовым, "+
			"потом сверяемся с правильным ответом.",
		b.kbStart(),
	)
}

func (b *Bot) onTrain(c tele.Context) error {
	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	inSession := s.State == session.StateInSession
	s.Unlock()

	if inSession {
		kb := &tele.ReplyMarkup{}
		kb.Inline(kb.Row(kb.Data("Да, начать заново", "train_yes"), kb.Data("Нет, продолжить", "train_no")))
		return c.Send("Идёт сессия. Начать новую?", kb)
	}
	return b.startSelection(c, s)
}

func (b *Bot) onStop(c tele.Context) error {
	s := b.store.Get(c.Chat().ID)
	if s != nil {
		s.Lock()
		defer s.Unlock()
		s.Reset()
		s.State = session.StateIdle
	}
	return c.Send("Сессия прервана. Набери /train, когда будешь готов продолжить.")
}

func (b *Bot) onHelp(c tele.Context) error {
	return c.Send(
		"/train — начать тренировку (язык → уровень → темы → количество → карточки)\n" +
			"/stop — прервать сессию\n" +
			"Отвечай голосовым 🎤 (или текстом), затем жми «Показать ответ» и сверяйся.",
	)
}

// --- Колбэки ---

func (b *Bot) onCallback(c tele.Context) error {
	data := c.Data()
	prefix, arg, _ := strings.Cut(data, ":")

	switch prefix {
	case "train":
		s := b.store.GetOrCreate(c.Chat().ID)
		s.Lock()
		defer s.Unlock()
		return b.startSelection(c, s)
	case "train_yes":
		s := b.store.GetOrCreate(c.Chat().ID)
		s.Lock()
		defer s.Unlock()
		return b.startSelection(c, s)
	case "train_no":
		return c.Respond(&tele.CallbackResponse{Text: "Продолжаем сессию"})
	case "lang":
		return b.selectLanguage(c, arg)
	case "level":
		return b.selectLevel(c, arg)
	case "topic":
		return b.toggleTopic(c, arg)
	case "topics_done":
		return b.finishTopics(c)
	case "topics_all":
		return b.selectAllTopics(c)
	case "count":
		return b.selectCount(c, arg)
	case "answer":
		return b.showAnswer(c)
	case "next":
		return b.nextCard(c)
	case "finish":
		return b.finishSession(c, "Сессия завершена досрочно.")
	default:
		return c.Respond(&tele.CallbackResponse{Text: "Неизвестное действие"})
	}
}

// startSelection — начало сценария: экран выбора языка.
func (b *Bot) startSelection(c tele.Context, s *session.Session) error {
	s.Reset() // State = SelectLanguage

	langs, err := b.api.ListLanguages(context.Background())
	if err != nil {
		return b.fail(c, err, "не удалось получить список языков")
	}

	kb := &tele.ReplyMarkup{}
	var rows []tele.Row
	row := kb.Row()
	for _, l := range langs {
		row = append(row, kb.Data(l.GetName(), "lang", l.GetCode()))
		if len(row) == 2 {
			rows = append(rows, row)
			row = kb.Row()
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	kb.Inline(rows...)

	text := "На какой язык переводим?"
	if c.Callback() != nil {
		return c.Edit(text, kb)
	}
	return c.Send(text, kb)
}

// selectLanguage — выбран язык.
func (b *Bot) selectLanguage(c tele.Context, code string) error {
	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	defer s.Unlock()

	s.LanguageCode = code
	s.LanguageName = langNames[code]
	s.State = session.StateSelectLevel

	levels, err := b.api.ListLevels(context.Background())
	if err != nil {
		return b.fail(c, err, "не удалось получить уровни")
	}

	kb := &tele.ReplyMarkup{}
	var rows []tele.Row
	row := kb.Row()
	for _, l := range levels {
		row = append(row, kb.Data(l.GetCode(), "level", l.GetCode()))
		if len(row) == 3 {
			rows = append(rows, row)
			row = kb.Row()
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	kb.Inline(rows...)

	return c.Edit(fmt.Sprintf("Выбран язык: %s. Какой уровень?", s.LanguageName), kb)
}

// selectLevel — выбран уровень; загружаем темы.
func (b *Bot) selectLevel(c tele.Context, level string) error {
	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	defer s.Unlock()

	s.LevelCode = level
	s.State = session.StateSelectTopics

	grammar, err := b.api.ListTopics(context.Background(), s.LanguageCode, true, s.LevelCode)
	if err != nil {
		return b.fail(c, err, "не удалось получить грамматические темы")
	}
	lexica, err := b.api.ListTopics(context.Background(), s.LanguageCode, false, "")
	if err != nil {
		return b.fail(c, err, "не удалось получить лексические темы")
	}

	s.Topics = s.Topics[:0]
	for _, t := range grammar {
		s.Topics = append(s.Topics, session.TopicChoice{ID: t.GetId(), Title: t.GetTitle(), IsGrammar: true})
	}
	for _, t := range lexica {
		s.Topics = append(s.Topics, session.TopicChoice{ID: t.GetId(), Title: t.GetTitle(), IsGrammar: false})
	}

	return c.Edit("Выбери темы (можно несколько, можно ничего = любые):", b.kbTopics(s))
}

// toggleTopic — переключение тумблера темы.
func (b *Bot) toggleTopic(c tele.Context, arg string) error {
	kind, idStr, _ := strings.Cut(arg, ":")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	defer s.Unlock()

	for i := range s.Topics {
		if s.Topics[i].ID == id && (kind == "g") == s.Topics[i].IsGrammar {
			s.Topics[i].Selected = !s.Topics[i].Selected
			break
		}
	}
	return c.Edit("Выбери темы (можно несколько, можно ничего = любые):", b.kbTopics(s))
}

// selectAllTopics — выбрать все темы.
func (b *Bot) selectAllTopics(c tele.Context) error {
	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	defer s.Unlock()

	for i := range s.Topics {
		s.Topics[i].Selected = true
	}
	return c.Edit("Выбери темы (можно несколько, можно ничего = любые):", b.kbTopics(s))
}

// finishTopics — «Готово»: экран количества карточек.
func (b *Bot) finishTopics(c tele.Context) error {
	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	selected := s.SelectedCount()
	s.Unlock()

	if selected == 0 {
		return c.Respond(&tele.CallbackResponse{Text: "Ни одна тема не выбрана — будут любые"})
	}
	s.Lock()
	s.State = session.StateSelectCount
	s.Unlock()

	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("5", "count", "5"), kb.Data("10", "count", "10"), kb.Data("15", "count", "15")),
	)
	return c.Edit("Сколько карточек потренируем?", kb)
}

// selectCount — запускаем сессию: тянем карточки из API.
func (b *Bot) selectCount(c tele.Context, arg string) error {
	var count int
	fmt.Sscanf(arg, "%d", &count)

	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	s.WantCount = count
	lang, level := s.LanguageCode, s.LevelCode
	gIDs := s.SelectedGrammarIDs()
	lIDs := s.SelectedLexicaIDs()
	s.Unlock()

	cards, err := b.api.DrawCards(context.Background(), lang, level, gIDs, lIDs, count)
	if err != nil {
		return b.fail(c, err, "не удалось получить карточки")
	}
	if len(cards) == 0 {
		s.Lock()
		s.Reset()
		s.State = session.StateIdle
		s.Unlock()
		return c.Edit("Под выбранный фильтр нет карточек. Попробуй другие темы: /train")
	}

	s.Lock()
	s.Cards = cards
	s.CurrentIndex = 0
	s.AnswerShown = false
	s.State = session.StateInSession
	s.Unlock()

	text := fmt.Sprintf("Тренируем %d карточек 🎯", len(cards))
	if len(cards) < count {
		text = fmt.Sprintf("Под фильтр нашлось только %d карточек (вместо %d) 🎯", len(cards), count)
	}
	return c.Edit(text+"\n\n"+b.cardPrompt(s), b.kbShowAnswer())
}

// showAnswer — показываем правильный перевод.
func (b *Bot) showAnswer(c tele.Context) error {
	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	defer s.Unlock()

	card := s.CurrentCard()
	if card == nil {
		return c.Respond(&tele.CallbackResponse{Text: "Нет активной карточки"})
	}
	s.AnswerShown = true

	text := fmt.Sprintf("✅ Правильный ответ:\n\n%s", card.GetTargetText())
	if card.GetNote() != "" {
		text += "\n\n📝 " + card.GetNote()
	}
	if topics := topicTitles(card); topics != "" {
		text += "\n" + topics
	}

	hasNext := s.CurrentIndex+1 < s.TotalCards()
	kb := &tele.ReplyMarkup{}
	if hasNext {
		kb.Inline(kb.Row(kb.Data("➡️ Следующая", "next"), kb.Data("🏁 Завершить", "finish")))
	} else {
		kb.Inline(kb.Row(kb.Data("🏁 Завершить", "finish")))
	}
	return c.Edit(text, kb)
}

// nextCard — следующая карточка или итоги.
func (b *Bot) nextCard(c tele.Context) error {
	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	defer s.Unlock()

	if !s.Advance() {
		return b.sendSummary(c, s, "")
	}
	return c.Edit(b.cardPrompt(s), b.kbShowAnswer())
}

// finishSession — досрочное завершение.
func (b *Bot) finishSession(c tele.Context, note string) error {
	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	defer s.Unlock()
	return b.sendSummary(c, s, note)
}

func (b *Bot) sendSummary(c tele.Context, s *session.Session, note string) error {
	s.State = session.StateSummary
	text := fmt.Sprintf("🎉 Готово! Проработано карточек: %d", s.CurrentIndex+1)
	if note != "" {
		text = note + "\n" + text
	}
	s.State = session.StateIdle
	return c.Edit(text, b.kbStart())
}

// --- Голос и текст ---

func (b *Bot) onVoice(c tele.Context) error {
	return b.acceptAnswer(c, "🎤 Голосовое принято!")
}

func (b *Bot) onText(c tele.Context) error {
	if strings.HasPrefix(c.Text(), "/") {
		return c.Send("Не знаю такую команду. /help — список команд.")
	}
	return b.acceptAnswer(c, "✍️ Ответ принят: «"+c.Text()+"»")
}

func (b *Bot) acceptAnswer(c tele.Context, ack string) error {
	s := b.store.GetOrCreate(c.Chat().ID)
	s.Lock()
	state := s.State
	s.Unlock()

	if state == session.StateInSession {
		return c.Send(ack + " Нажми «Показать ответ», чтобы свериться.")
	}
	if state == session.StateIdle || state == session.StateSummary {
		return c.Send("Записывай ответ, когда появится карточка. Начать: /train")
	}
	return c.Send("Сначала закончи выбор: язык → уровень → темы → количество.")
}

// --- Хелперы ---

// cardPrompt — текст задания для текущей карточки.
func (b *Bot) cardPrompt(s *session.Session) string {
	card := s.CurrentCard()
	if card == nil {
		return ""
	}
	return fmt.Sprintf(
		"🎤 Карточка %d из %d. Скажи по-%s:\n\n%s",
		s.CurrentIndex+1, s.TotalCards(), s.LanguageName, card.GetSourceText(),
	)
}

func topicTitles(card *trainerv1.Card) string {
	var parts []string
	for _, t := range card.GetGrammarTopics() {
		parts = append(parts, "[Г] "+t.GetTitle())
	}
	for _, t := range card.GetLexicaTopics() {
		parts = append(parts, "[Л] "+t.GetTitle())
	}
	if len(parts) == 0 {
		return ""
	}
	return "🏷 " + strings.Join(parts, ", ")
}

func (b *Bot) kbTopics(s *session.Session) *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	var rows []tele.Row
	row := kb.Row()
	for _, t := range s.Topics {
		mark := "◻️"
		if t.Selected {
			mark = "☑️"
		}
		prefix := "l"
		if t.IsGrammar {
			prefix = "g"
		}
		row = append(row, kb.Data(mark+" "+t.Title, "topic", fmt.Sprintf("%s:%d", prefix, t.ID)))
		if len(row) == 2 {
			rows = append(rows, row)
			row = kb.Row()
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, kb.Row(kb.Data("Выбрать все", "topics_all")), kb.Row(kb.Data("✅ Готово", "topics_done")))
	kb.Inline(rows...)
	return kb
}

func (b *Bot) kbStart() *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("💪 Начать тренировку", "train")))
	return kb
}

func (b *Bot) kbShowAnswer() *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("👀 Показать ответ", "answer")))
	return kb
}

// fail — логируем ошибку и сообщаем пользователю.
func (b *Bot) fail(c tele.Context, err error, userMsg string) error {
	b.log.Error("bot error", "err", err, "chat", c.Chat().ID)
	return c.Send("⚠️ " + userMsg + ". Попробуй ещё раз или /start.")
}

// langNames — названия языков для текстов заданий (fallback — код).
var langNames = map[string]string{
	"it": "итальянский",
	"en": "английский",
}
