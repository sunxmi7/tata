package service

import (
	"context"

	"tata/server/internal/dao"
	"tata/server/internal/model/entity"
)

func ListSpreads(ctx context.Context) ([]entity.Spreads, error) {
	var rows []entity.Spreads
	err := dao.Spreads.Ctx(ctx).OrderAsc(dao.Spreads.Columns().Id).Scan(&rows)
	return rows, err
}
