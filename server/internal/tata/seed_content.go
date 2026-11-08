package tata

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
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
	Scene      string `json:"scene"`
	Category   string `json:"category"`
	Body       string `json:"body"`
	SortOrder  int    `json:"sort_order"`
}

var presetSceneDefs = []struct{ scene, title string }{
	{"relationship", "感情关系"},
	{"career", "事业与工作"},
	{"study", "学业与考试"},
	{"growth", "自我成长"},
	{"social", "人际关系"},
	{"life", "生活与选择"},
}

func knownPresetScene(scene string) bool {
	for _, d := range presetSceneDefs {
		if d.scene == scene {
			return true
		}
	}
	return false
}

func sceneFromCategory(category string) string {
	for _, d := range presetSceneDefs {
		if d.title == category {
			return d.scene
		}
	}
	return ""
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
		scene := strings.TrimSpace(q.Scene)
		if scene == "" {
			scene = sceneFromCategory(q.Category)
		}
		if _, err := s.Exec(ctx, `INSERT INTO preset_questions(question_id, pool, scene, category, body, sort_order, enabled)
			VALUES ($1,$2,$3,$4,$5,$6,true)
			ON CONFLICT (question_id) DO UPDATE SET
				scene = CASE WHEN preset_questions.scene = '' THEN EXCLUDED.scene ELSE preset_questions.scene END,
				category = CASE WHEN preset_questions.category = '' THEN EXCLUDED.category ELSE preset_questions.category END`,
			q.QuestionID, q.Pool, scene, q.Category, q.Body, q.SortOrder); err != nil {
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
	scene := strings.TrimSpace(r.URL.Query().Get("scene"))
	if scene != "" && !knownPresetScene(scene) {
		return biz("场景不正确")
	}
	q := `SELECT question_id, pool, scene, category, body, sort_order
		FROM preset_questions WHERE enabled=true`
	args := []any{}
	if scene != "" {
		args = append(args, scene)
		q += ` AND scene=$1`
	}
	q += ` ORDER BY pool, sort_order, question_id`
	rows, err := a.Store.Query(r.Context(), q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	featured := []map[string]any{}
	backup := []map[string]any{}
	for rows.Next() {
		var id, pool, rowScene, category, body string
		var order int
		if err := rows.Scan(&id, &pool, &rowScene, &category, &body, &order); err != nil {
			return err
		}
		item := map[string]any{
			"question_id": id,
			"scene":       rowScene,
			"category":    T(r.Context(), category),
			"body":        T(r.Context(), body),
			"sort_order":  order,
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
	scenes := make([]map[string]any, 0, len(presetSceneDefs))
	for _, d := range presetSceneDefs {
		scenes = append(scenes, map[string]any{"scene": d.scene, "title": T(r.Context(), d.title)})
	}
	writeOK(w, map[string]any{
		"featured":      featured,
		"backup":        backup,
		"scenes":        scenes,
		"more_per_page": more,
	})
	return nil
}
