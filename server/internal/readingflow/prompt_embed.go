package readingflow

import _ "embed"

//go:embed prompts/common.md
var promptCommon string

//go:embed prompts/normal.md
var promptNormal string

//go:embed prompts/daily_today.md
var promptDailyToday string

//go:embed prompts/daily_tomorrow.md
var promptDailyTomorrow string

//go:embed prompts/explore.md
var promptExplore string
