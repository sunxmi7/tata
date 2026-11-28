package tata

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const adminUserSelectCols = `id, email, wechat_openid, apple_sub, phone,
		COALESCE(NULLIF(timezone,''),'Asia/Shanghai'), tarotist_trial_used, tease_off, cancelled_at, created_at,
		COALESCE(name,''), COALESCE(avatar_url,''), gender, birth_date, COALESCE(explore_preferences,'[]'),
		merged_into, COALESCE(register_ip,''), COALESCE(region_province,''), COALESCE(region_city,''), COALESCE(region_status,''),
		EXISTS(
			SELECT 1 FROM subscriptions s
			WHERE s.user_id = app_users.id
			  AND s.status = 'active'
			  AND s.expires_at > now()
			  AND s.merge_state NOT IN ('superseded','expired')
		)`

func (a *App) adminListUsers(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	page, err := parseAdminPage(r)
	if err != nil {
		return err
	}
	where, args, err := buildAdminUserListFilter(r)
	if err != nil {
		return err
	}
	var total int
	if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM app_users WHERE `+where, args...).Scan(&total); err != nil {
		return err
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := a.Store.Query(r.Context(), `SELECT `+adminUserSelectCols+`
		FROM app_users WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		item, err := scanAdminUser(r, rows)
		if err != nil {
			return err
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{"items": list, "total": total, "limit": page.Limit, "offset": page.Offset})
	return nil
}

func (a *App) adminExportUsers(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, false)
	if err != nil {
		return err
	}
	where, args, err := buildAdminUserListFilter(r)
	if err != nil {
		return err
	}
	args = append(args, 10000)
	rows, err := a.Store.Query(r.Context(), `SELECT `+adminUserSelectCols+`
		FROM app_users WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	csvRows := [][]string{}
	for rows.Next() {
		item, err := scanAdminUser(r, rows)
		if err != nil {
			return err
		}
		member := "否"
		if item["is_member"] == true {
			member = "是"
		}
		csvRows = append(csvRows, []string{
			fmt.Sprint(item["id"]),
			fmt.Sprint(item["name"]),
			nullCSV(item["email"]),
			nullCSV(item["phone"]),
			nullCSV(item["wechat_openid"]),
			nullCSV(item["apple_sub"]),
			fmt.Sprint(item["gender"]),
			nullCSV(item["birth_date"]),
			fmt.Sprint(item["timezone"]),
			fmt.Sprint(item["region_province"]),
			fmt.Sprint(item["region_city"]),
			fmt.Sprint(item["register_ip"]),
			member,
			nullCSV(item["cancelled_at"]),
			nullCSV(item["merged_into"]),
			fmt.Sprint(item["created_at"]),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "export", "app_user", "", fmt.Sprintf("rows=%d", len(csvRows)))
	return writeCSV(w, "users.csv", []string{
		"id", "name", "email", "phone", "wechat_openid", "apple_sub", "gender", "birth_date",
		"timezone", "region_province", "region_city", "register_ip", "is_member", "cancelled_at", "merged_into", "created_at",
	}, csvRows)
}

func (a *App) adminGetUser(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	row := a.Store.QueryRow(r.Context(), `SELECT `+adminUserSelectCols+`
		FROM app_users WHERE id=$1`, id)
	user, err := scanAdminUser(r, row)
	if err == sql.ErrNoRows {
		return biz("用户不存在")
	}
	if err != nil {
		return err
	}

	readings := []map[string]any{}
	rrows, err := a.Store.Query(r.Context(), `SELECT id, type, question, status, created_at, updated_at
		FROM readings WHERE user_id=$1 ORDER BY id DESC LIMIT 10`, id)
	if err != nil {
		return err
	}
	defer rrows.Close()
	for rrows.Next() {
		var rid int64
		var typ, question, status string
		var created, updated time.Time
		if err := rrows.Scan(&rid, &typ, &question, &status, &created, &updated); err != nil {
			return err
		}
		label, _ := progressOf(status)
		readings = append(readings, map[string]any{
			"id": rid, "type": typ, "question": question, "status": status,
			"stage_label": label, "created_at": created, "updated_at": updated,
		})
	}
	if err := rrows.Err(); err != nil {
		return err
	}

	orders := []map[string]any{}
	orows, err := a.Store.Query(r.Context(), `SELECT o.id, o.sku_code, o.sku_kind, o.platform, o.verify_status, o.credit_status,
		o.created_at, o.verified_at,
		COALESCE(o.price_amount_minor, s.price_amount_minor),
		COALESCE(NULLIF(o.price_currency,''), NULLIF(s.price_currency,''), ''),
		(o.price_amount_minor IS NULL)
		FROM iap_orders o
		LEFT JOIN iap_skus s ON s.id = o.sku_id
		WHERE o.user_id=$1 ORDER BY o.id DESC LIMIT 10`, id)
	if err != nil {
		return err
	}
	defer orows.Close()
	for orows.Next() {
		var oid int64
		var skuCode, skuKind, platform, verifyStatus, creditStatus, currency string
		var created time.Time
		var verified sql.NullTime
		var price sql.NullInt64
		var estimated bool
		if err := orows.Scan(&oid, &skuCode, &skuKind, &platform, &verifyStatus, &creditStatus, &created, &verified, &price, &currency, &estimated); err != nil {
			return err
		}
		orders = append(orders, map[string]any{
			"id": oid, "sku_code": skuCode, "sku_kind": skuKind, "platform": platform,
			"verify_status": verifyStatus, "credit_status": creditStatus,
			"created_at": created, "verified_at": nullTime(verified),
			"price_amount_minor": nullInt(price), "price_currency": currency, "price_estimated": estimated && price.Valid,
		})
	}
	if err := orows.Err(); err != nil {
		return err
	}

	writeOK(w, map[string]any{
		"user":            user,
		"recent_readings": readings,
		"recent_orders":   orders,
		"notes":           []string{"会员/额度/赠送/邀请/年运请用现有权益接口按 user_id 查询", "功能使用仅含业务单据，不含点击轨迹"},
	})
	return nil
}

func buildAdminUserListFilter(r *http.Request) (where string, args []any, err error) {
	conds := []string{"1=1"}
	if raw := strings.TrimSpace(r.URL.Query().Get("id")); raw != "" {
		id, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || id <= 0 {
			return "", nil, biz("用户 id 不正确")
		}
		args = append(args, id)
		conds = append(conds, fmt.Sprintf("id=$%d", len(args)))
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		like := "%" + q + "%"
		args = append(args, like, like, like)
		n := len(args)
		cond := fmt.Sprintf(`(COALESCE(name,'') ILIKE $%d OR COALESCE(email,'') ILIKE $%d OR COALESCE(phone,'') ILIKE $%d`, n-2, n-1, n)
		if id, perr := strconv.ParseInt(q, 10, 64); perr == nil && id > 0 {
			args = append(args, id)
			cond += fmt.Sprintf(` OR id=$%d`, len(args))
		}
		conds = append(conds, cond+")")
	}
	if set, val, berr := queryBoolFilter(r.URL.Query().Get("cancelled")); berr != nil {
		return "", nil, berr
	} else if set {
		if val {
			conds = append(conds, "cancelled_at IS NOT NULL")
		} else {
			conds = append(conds, "cancelled_at IS NULL")
		}
	}
	if set, val, berr := queryBoolFilter(r.URL.Query().Get("merged")); berr != nil {
		return "", nil, berr
	} else if set {
		if val {
			conds = append(conds, "merged_into IS NOT NULL")
		} else {
			conds = append(conds, "merged_into IS NULL")
		}
	}
	return strings.Join(conds, " AND "), args, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanAdminUser(r *http.Request, row scannable) (map[string]any, error) {
	var id int64
	var email, openid, apple, phone, gender, birth *string
	var tz, name, avatar, prefs, registerIP, province, city, regionStatus string
	var trial, tease, isMember bool
	var cancelled *time.Time
	var created time.Time
	var merged *int64
	if err := row.Scan(&id, &email, &openid, &apple, &phone, &tz, &trial, &tease, &cancelled, &created,
		&name, &avatar, &gender, &birth, &prefs, &merged, &registerIP, &province, &city, &regionStatus, &isMember); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "email": email, "wechat_openid": openid, "apple_sub": apple, "phone": phone,
		"timezone": tz, "tarotist_trial_used": trial, "tease_off": tease,
		"cancelled_at": cancelled, "created_at": created,
		"name": name, "nickname": name, "avatar_url": profileAvatarURL(r, avatar),
		"gender": gender, "birth_date": birth, "explore_preferences": parseExplorePreferences(prefs),
		"merged_into": merged, "register_ip": registerIP,
		"region_province": province, "region_city": city, "region_status": regionStatus,
		"is_member": isMember,
	}, nil
}

func nullCSV(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case *string:
		if t == nil {
			return ""
		}
		return *t
	case *int64:
		if t == nil {
			return ""
		}
		return strconv.FormatInt(*t, 10)
	case *time.Time:
		if t == nil {
			return ""
		}
		return t.Format(time.RFC3339)
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return fmt.Sprint(v)
	}
}

func (a *App) adminListAudit(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	page, err := parseAdminPage(r)
	if err != nil {
		return err
	}
	where, args, err := adminAuditFilter(r)
	if err != nil {
		return err
	}
	var total int
	if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM audit_log WHERE `+where, args...).Scan(&total); err != nil {
		return err
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := a.Store.Query(r.Context(), `SELECT id, operator_id, action, object_type, object_id, detail, created_at, actor_kind
		FROM audit_log WHERE `+where+` ORDER BY id DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, op int64
		var action, objectType, objectID, detail, actorKind string
		var created time.Time
		if err := rows.Scan(&id, &op, &action, &objectType, &objectID, &detail, &created, &actorKind); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "operator_id": op, "action": action, "object_type": objectType,
			"object_id": objectID, "detail": detail, "actor_kind": actorKind, "created_at": created,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{"items": list, "total": total, "limit": page.Limit, "offset": page.Offset})
	return nil
}

func (a *App) adminExportAudit(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, false)
	if err != nil {
		return err
	}
	where, args, err := adminAuditFilter(r)
	if err != nil {
		return err
	}
	args = append(args, 10000)
	rows, err := a.Store.Query(r.Context(), `SELECT id, operator_id, action, object_type, object_id, detail, created_at, actor_kind
		FROM audit_log WHERE `+where+` ORDER BY id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	csvRows := [][]string{}
	for rows.Next() {
		var id, op int64
		var action, objectType, objectID, detail, actorKind string
		var created time.Time
		if err := rows.Scan(&id, &op, &action, &objectType, &objectID, &detail, &created, &actorKind); err != nil {
			return err
		}
		csvRows = append(csvRows, []string{
			strconv.FormatInt(id, 10), strconv.FormatInt(op, 10), action, objectType, objectID, detail, actorKind, created.Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "export", "audit_log", "", fmt.Sprintf("rows=%d", len(csvRows)))
	return writeCSV(w, "audit.csv", []string{"id", "operator_id", "action", "object_type", "object_id", "detail", "actor_kind", "created_at"}, csvRows)
}

func adminAuditFilter(r *http.Request) (where string, args []any, err error) {
	conds := []string{"1=1"}
	if raw := strings.TrimSpace(r.URL.Query().Get("operator_id")); raw != "" {
		id, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || id <= 0 {
			return "", nil, biz("操作员 id 不正确")
		}
		args = append(args, id)
		conds = append(conds, fmt.Sprintf("operator_id=$%d", len(args)))
	}
	if v := strings.TrimSpace(r.URL.Query().Get("action")); v != "" {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf("action=$%d", len(args)))
	}
	if v := strings.TrimSpace(r.URL.Query().Get("object_type")); v != "" {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf("object_type=$%d", len(args)))
	}
	from, to, terr := parseTimeRange(r, "from", "to")
	if terr != nil {
		return "", nil, terr
	}
	if from != nil {
		args = append(args, *from)
		conds = append(conds, fmt.Sprintf("created_at>=$%d", len(args)))
	}
	if to != nil {
		args = append(args, *to)
		conds = append(conds, fmt.Sprintf("created_at<=$%d", len(args)))
	}
	return strings.Join(conds, " AND "), args, nil
}

func (a *App) adminIAPRevenue(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	from, to, err := parseTimeRange(r, "from", "to")
	if err != nil {
		return err
	}
	grain := strings.TrimSpace(r.URL.Query().Get("grain"))
	if grain == "" {
		grain = "day"
	}
	var trunc string
	switch grain {
	case "day":
		trunc = "day"
	case "week":
		trunc = "week"
	case "month":
		trunc = "month"
	default:
		return biz("统计粒度不正确")
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	if from == nil {
		t := beijingNow().AddDate(0, 0, -29)
		start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		from = &start
	}
	if to == nil {
		n := beijingNow()
		end := time.Date(n.Year(), n.Month(), n.Day(), 23, 59, 59, 0, loc)
		to = &end
	}

	amountExpr := `COALESCE(o.price_amount_minor, s.price_amount_minor, 0)`
	var revenue int64
	var payingUsers int
	err = a.Store.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(`+amountExpr+`),0), COUNT(DISTINCT o.user_id)
		FROM iap_orders o
		LEFT JOIN iap_skus s ON s.id = o.sku_id
		WHERE o.verify_status='verified' AND o.created_at>=$1 AND o.created_at<=$2`, *from, *to).Scan(&revenue, &payingUsers)
	if err != nil {
		return err
	}
	var newUsers int
	err = a.Store.QueryRow(r.Context(), `
		SELECT count(*) FROM app_users
		WHERE merged_into IS NULL AND created_at>=$1 AND created_at<=$2`, *from, *to).Scan(&newUsers)
	if err != nil {
		return err
	}
	payRate := 0.0
	if newUsers > 0 {
		payRate = float64(payingUsers) * 100 / float64(newUsers)
		payRate = float64(int(payRate*10+0.5)) / 10
	}

	skuRows, err := a.Store.Query(r.Context(), `
		SELECT o.sku_code, o.sku_kind, COUNT(*)::int, COUNT(DISTINCT o.user_id)::int, COALESCE(SUM(`+amountExpr+`),0)
		FROM iap_orders o
		LEFT JOIN iap_skus s ON s.id = o.sku_id
		WHERE o.verify_status='verified' AND o.created_at>=$1 AND o.created_at<=$2
		GROUP BY o.sku_code, o.sku_kind
		ORDER BY COALESCE(SUM(`+amountExpr+`),0) DESC, o.sku_code`, *from, *to)
	if err != nil {
		return err
	}
	defer skuRows.Close()
	bySKU := []map[string]any{}
	for skuRows.Next() {
		var code, kind string
		var orders, users int
		var amount int64
		if err := skuRows.Scan(&code, &kind, &orders, &users, &amount); err != nil {
			return err
		}
		bySKU = append(bySKU, map[string]any{
			"sku_code": code, "sku_kind": kind, "orders": orders, "paying_users": users, "revenue_minor": amount,
		})
	}
	if err := skuRows.Err(); err != nil {
		return err
	}

	seriesRows, err := a.Store.Query(r.Context(), `
		SELECT to_char(date_trunc($3, o.created_at AT TIME ZONE 'Asia/Shanghai'), 'YYYY-MM-DD') AS bucket,
			COUNT(*)::int, COUNT(DISTINCT o.user_id)::int, COALESCE(SUM(`+amountExpr+`),0)
		FROM iap_orders o
		LEFT JOIN iap_skus s ON s.id = o.sku_id
		WHERE o.verify_status='verified' AND o.created_at>=$1 AND o.created_at<=$2
		GROUP BY 1
		ORDER BY 1`, *from, *to, trunc)
	if err != nil {
		return err
	}
	defer seriesRows.Close()
	series := []map[string]any{}
	for seriesRows.Next() {
		var bucket string
		var orders, users int
		var amount int64
		if err := seriesRows.Scan(&bucket, &orders, &users, &amount); err != nil {
			return err
		}
		series = append(series, map[string]any{
			"bucket": bucket, "orders": orders, "paying_users": users, "revenue_minor": amount,
		})
	}
	if err := seriesRows.Err(); err != nil {
		return err
	}

	writeOK(w, map[string]any{
		"from": from, "to": to, "grain": grain,
		"revenue_minor": revenue, "paying_users": payingUsers, "new_users": newUsers, "pay_rate": payRate,
		"by_sku": bySKU, "series": series,
		"amount_basis": "order_snapshot_or_sku",
		"currency":     "CNY",
		"notes": []string{
			"收入=验单成功订单金额之和；订单无快照时按当前 SKU 价估算",
			"付费人数=统计期内至少一笔验单成功的去重用户",
			"付费率=付费人数/同期新增（未合并账号）",
			"业务日按 Asia/Shanghai",
		},
	})
	return nil
}
