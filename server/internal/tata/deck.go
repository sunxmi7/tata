package tata

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

type position struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Meaning string `json:"meaning"`
}

// Rider-Waite order: Strength is VIII, Justice is XI.
var majors = []string{
	"愚者", "魔术师", "女祭司", "女皇", "皇帝", "教皇", "恋人", "战车",
	"力量", "隐者", "命运之轮", "正义", "倒吊人", "死神", "节制", "恶魔",
	"高塔", "星星", "月亮", "太阳", "审判", "世界",
}

var suits = []string{"权杖", "圣杯", "宝剑", "星币"}
var ranks = []string{"王牌", "二", "三", "四", "五", "六", "七", "八", "九", "十", "侍从", "骑士", "王后", "国王"}

type cardDef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func deck() []cardDef {
	out := make([]cardDef, 0, 78)
	for i, name := range majors {
		out = append(out, cardDef{ID: i, Name: name})
	}
	for _, suit := range suits {
		for _, rank := range ranks {
			out = append(out, cardDef{ID: len(out), Name: suit + rank})
		}
	}
	if len(out) != 78 {
		panic(fmt.Sprintf("deck size %d", len(out)))
	}
	if out[8].Name != "力量" || out[11].Name != "正义" {
		panic("rider-waite order broken")
	}
	return out
}

type drawnCard struct {
	Position     int    `json:"position"`
	PositionName string `json:"position_name"`
	CardID       int    `json:"card_id"`
	Name         string `json:"name"`
	Orientation  string `json:"orientation"`
}

func drawCards(positions []position) ([]drawnCard, error) {
	all := deck()
	order := make([]int, len(all))
	for i := range order {
		order[i] = i
	}
	for i := len(order) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return nil, err
		}
		j := int(n.Int64())
		order[i], order[j] = order[j], order[i]
	}
	out := make([]drawnCard, len(positions))
	for i, pos := range positions {
		bit, err := rand.Int(rand.Reader, big.NewInt(2))
		if err != nil {
			return nil, err
		}
		ori := "upright"
		if bit.Int64() == 1 {
			ori = "reversed"
		}
		c := all[order[i]]
		out[i] = drawnCard{
			Position:     pos.Index,
			PositionName: pos.Name,
			CardID:       c.ID,
			Name:         c.Name,
			Orientation:  ori,
		}
	}
	return out, nil
}
