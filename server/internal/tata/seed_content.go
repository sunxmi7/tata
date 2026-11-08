package tata

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
)

//go:embed seed_content.json
var seedContentJSON []byte

type seedContentFile struct {
	Cards       []seedCard     `json:"cards"`
	Questions   []seedQuestion `json:"questions"`
	MorePerPage int            `json:"more_per_page"`
	Deck        seedDeckNames  `json:"deck"`
}

type seedCard struct {
	Name string `json:"name"`
	File string `json:"file"`
	EN   string `json:"en"`
	JA   string `json:"ja"`
	KO   string `json:"ko"`
}

type seedQuestion struct {
	QuestionID string `json:"question_id"`
	Pool       string `json:"pool"`
	Category   string `json:"category"`
	Body       string `json:"body"`
	SortOrder  int    `json:"sort_order"`
}

type seedDeckNames struct {
	Code   string `json:"code"`
	NameZH string `json:"name_zh"`
	NameEN string `json:"name_en"`
	NameJA string `json:"name_ja"`
	NameKO string `json:"name_ko"`
}

func loadSeedContent() seedContentFile {
	var out seedContentFile
	if err := json.Unmarshal(seedContentJSON, &out); err != nil {
		panic("seed_content.json: " + err.Error())
	}
	return out
}

func (s *Store) seedContentV1(ctx context.Context) error {
	data := loadSeedContent()
	for _, q := range data.Questions {
		if _, err := s.Exec(ctx, `INSERT INTO preset_questions(question_id, pool, category, body, sort_order, enabled)
			VALUES ($1,$2,$3,$4,$5,true)
			ON CONFLICT (question_id) DO NOTHING`,
			q.QuestionID, q.Pool, q.Category, q.Body, q.SortOrder); err != nil {
			return err
		}
	}
	for _, c := range data.Cards {
		thumb := "/uploads/cards/" + c.File
		if err := s.putConfigIfEmpty(ctx, cardThumbnailKey(c.Name), thumb); err != nil {
			return err
		}
		if c.EN != "" {
			if err := s.putConfigIfEmpty(ctx, "card."+c.Name+".name_en", c.EN); err != nil {
				return err
			}
		}
		if c.JA != "" {
			if err := s.putConfigIfEmpty(ctx, "card."+c.Name+".name_ja", c.JA); err != nil {
				return err
			}
		}
		if c.KO != "" {
			if err := s.putConfigIfEmpty(ctx, "card."+c.Name+".name_ko", c.KO); err != nil {
				return err
			}
		}
	}
	d := data.Deck
	if d.NameZH != "" {
		if err := s.putConfigIfEmpty(ctx, "deck.rider-waite.name_zh", d.NameZH); err != nil {
			return err
		}
	}
	if d.NameEN != "" {
		if err := s.putConfigIfEmpty(ctx, "deck.rider-waite.name_en", d.NameEN); err != nil {
			return err
		}
	}
	if d.NameJA != "" {
		if err := s.putConfigIfEmpty(ctx, "deck.rider-waite.name_ja", d.NameJA); err != nil {
			return err
		}
	}
	if d.NameKO != "" {
		if err := s.putConfigIfEmpty(ctx, "deck.rider-waite.name_ko", d.NameKO); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) putConfigIfEmpty(ctx context.Context, key, value string) error {
	_, err := s.Exec(ctx, `INSERT INTO app_config(key, value, updated_at) VALUES ($1,$2,now())
		ON CONFLICT (key) DO NOTHING`, key, value)
	return err
}

func (a *App) appPresetQuestions(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT question_id, pool, category, body, sort_order
		FROM preset_questions WHERE enabled=true ORDER BY pool, sort_order, question_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	featured := []map[string]any{}
	backup := []map[string]any{}
	for rows.Next() {
		var id, pool, category, body string
		var order int
		if err := rows.Scan(&id, &pool, &category, &body, &order); err != nil {
			return err
		}
		item := map[string]any{"question_id": id, "body": T(r.Context(), body), "sort_order": order}
		if category != "" {
			item["category"] = T(r.Context(), category)
		}
		if pool == "featured" {
			featured = append(featured, item)
		} else {
			backup = append(backup, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	more := 5
	if n := loadSeedContent().MorePerPage; n > 0 {
		more = n
	}
	writeOK(w, map[string]any{
		"featured":      featured,
		"backup":        backup,
		"more_per_page": more,
	})
	return nil
}
