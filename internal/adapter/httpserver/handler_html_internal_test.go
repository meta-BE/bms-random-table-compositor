package httpserver

import (
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

// TestFormatRelativeDuration_Boundaries は相対日時フォーマッタの境界値テスト。
// 秒/分/時間/日/ヶ月(30日)/年(365日) の 6 単位境界と負値クランプを網羅する。
func TestFormatRelativeDuration_Boundaries(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"negative clamped", -time.Hour, "0秒前"},
		{"0秒", 0, "0秒前"},
		{"30秒", 30 * time.Second, "30秒前"},
		{"59秒", 59 * time.Second, "59秒前"},
		{"60秒 = 1分", 60 * time.Second, "1分前"},
		{"30分", 30 * time.Minute, "30分前"},
		{"59分", 59 * time.Minute, "59分前"},
		{"60分 = 1時間", 60 * time.Minute, "1時間前"},
		{"23時間", 23 * time.Hour, "23時間前"},
		{"24時間 = 1日", 24 * time.Hour, "1日前"},
		{"29日", 29 * 24 * time.Hour, "29日前"},
		{"30日 = 1ヶ月", 30 * 24 * time.Hour, "1ヶ月前"},
		{"364日", 364 * 24 * time.Hour, "12ヶ月前"},
		{"365日 = 1年", 365 * 24 * time.Hour, "1年前"},
		{"2年", 2 * 365 * 24 * time.Hour, "2年前"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := formatRelativeDuration(c.d)
			require.Equal(t, c.want, got)
		})
	}
}

// TestBuildHTMLPageData_LastPlayed_Unplayed は LastPlayedAt が nil の場合に "未プレイ" になることを検証。
func TestBuildHTMLPageData_LastPlayed_Unplayed(t *testing.T) {
	now := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	r := domain.PickResult{
		GeneratedAt: now,
		LevelOrder:  []string{"0"},
		Charts: []domain.PickedChart{
			{EnrichedChart: domain.EnrichedChart{
				SourceChart: domain.SourceChart{Level: "0", Title: "未プレイ曲", MD5: "m1"},
			}, PublicLevel: "0"},
		},
	}
	pub := domain.PublishedTable{Slug: "s", DisplayName: "D", Symbol: "★"}
	data := buildHTMLPageData(pub, r)
	require.Equal(t, "未プレイ", data.Levels[0].Charts[0].LastPlayed)
}

// TestBuildHTMLPageData_LastPlayed_Recent は LastPlayedAt が指定された場合に相対表記で表示されることを検証。
// 基準時刻は r.GeneratedAt。
func TestBuildHTMLPageData_LastPlayed_Recent(t *testing.T) {
	now := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	played := now.Add(-3 * 24 * time.Hour)
	r := domain.PickResult{
		GeneratedAt: now,
		LevelOrder:  []string{"0"},
		Charts: []domain.PickedChart{
			{EnrichedChart: domain.EnrichedChart{
				SourceChart:  domain.SourceChart{Level: "0", Title: "3日前", MD5: "m1"},
				LastPlayedAt: &played,
			}, PublicLevel: "0"},
		},
	}
	pub := domain.PublishedTable{Slug: "s", DisplayName: "D", Symbol: "★"}
	data := buildHTMLPageData(pub, r)
	require.Equal(t, "3日前", data.Levels[0].Charts[0].LastPlayed)
}

// TestFormatPickSummary は更新タイミング・重み付け・所持限定の組み合わせを網羅。
func TestFormatPickSummary(t *testing.T) {
	cases := []struct {
		name string
		pub  domain.PublishedTable
		want string
	}{
		{
			name: "daily + probability + owned",
			pub: domain.PublishedTable{
				OwnedOnly: true,
				Pick: domain.PickConfig{
					RefreshMode: domain.RefreshModeDaily, WeightMode: domain.WeightModeProbability, WeightParamX: 10,
				},
			},
			want: "更新: 毎日 / 重み付け: 古い曲ほど優先 (確率 X=10) / 所持限定",
		},
		{
			name: "per_request + off + not owned",
			pub: domain.PublishedTable{
				Pick: domain.PickConfig{
					RefreshMode: domain.RefreshModePerRequest, WeightMode: domain.WeightModeOff,
				},
			},
			want: "更新: リクエスト毎 / 重み付け: 重み付けなし",
		},
		{
			name: "manual + sort + owned",
			pub: domain.PublishedTable{
				OwnedOnly: true,
				Pick: domain.PickConfig{
					RefreshMode: domain.RefreshModeManual, WeightMode: domain.WeightModeSort,
				},
			},
			want: "更新: 手動 / 重み付け: 最終プレイが古い順 / 所持限定",
		},
		{
			name: "empty WeightMode は off 扱い",
			pub: domain.PublishedTable{
				Pick: domain.PickConfig{RefreshMode: domain.RefreshModeDaily},
			},
			want: "更新: 毎日 / 重み付け: 重み付けなし",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, formatPickSummary(c.pub))
		})
	}
}
