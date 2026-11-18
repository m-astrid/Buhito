-- +goose Up
CREATE TABLE language (
    id   INTEGER PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);

CREATE TABLE level (
    id         INTEGER PRIMARY KEY,
    code       TEXT NOT NULL UNIQUE,
    sort_order INTEGER NOT NULL
);

CREATE TABLE grammar_topic (
    id          INTEGER PRIMARY KEY,
    language_id INTEGER NOT NULL REFERENCES language (id) ON DELETE CASCADE,
    level_id    INTEGER REFERENCES level (id) ON DELETE SET NULL, -- NULL = для любого уровня
    title       TEXT NOT NULL,
    UNIQUE (language_id, title)
);

CREATE TABLE lexica_topic (
    id          INTEGER PRIMARY KEY,
    language_id INTEGER NOT NULL REFERENCES language (id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    UNIQUE (language_id, title)
);

CREATE TABLE card (
    id          INTEGER PRIMARY KEY,
    language_id INTEGER NOT NULL REFERENCES language (id) ON DELETE CASCADE,
    level_id    INTEGER NOT NULL REFERENCES level (id),
    source_text TEXT NOT NULL,           -- RU-фраза
    target_text TEXT NOT NULL,           -- правильный перевод
    note        TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_card_language_level ON card (language_id, level_id);

CREATE TABLE card_grammar_topic (
    card_id          INTEGER NOT NULL REFERENCES card (id) ON DELETE CASCADE,
    grammar_topic_id INTEGER NOT NULL REFERENCES grammar_topic (id) ON DELETE CASCADE,
    PRIMARY KEY (card_id, grammar_topic_id)
);

CREATE TABLE card_lexica_topic (
    card_id         INTEGER NOT NULL REFERENCES card (id) ON DELETE CASCADE,
    lexica_topic_id INTEGER NOT NULL REFERENCES lexica_topic (id) ON DELETE CASCADE,
    PRIMARY KEY (card_id, lexica_topic_id)
);

CREATE INDEX idx_cgt_topic ON card_grammar_topic (grammar_topic_id);
CREATE INDEX idx_clt_topic ON card_lexica_topic (lexica_topic_id);

-- Справочные данные: целевые языки (родной всегда русский).
INSERT INTO language (code, name) VALUES
    ('it', 'Итальянский'),
    ('en', 'Английский');

-- Справочные данные: уровни CEFR.
INSERT INTO level (code, sort_order) VALUES
    ('A1', 1),
    ('A2', 2),
    ('B1', 3),
    ('B2', 4),
    ('C1', 5);

-- +goose Down
DROP TABLE card_lexica_topic;
DROP TABLE card_grammar_topic;
DROP TABLE card;
DROP TABLE lexica_topic;
DROP TABLE grammar_topic;
DROP TABLE level;
DROP TABLE language;
