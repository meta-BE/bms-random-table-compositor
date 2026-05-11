package weighter

import (
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// TestSL0Simulation_ProbabilityDistribution_X10 は testdata の score.db を実際に読んで、
// sl0 (satellite level=0) の所持・プレイ済み曲集合で probability X=10 のときの
// 「最古/中央値/最新」の選出確率がブレインストーミング時の数値 (1.13%, 0.66%, 0.11%) と
// 整合することを回帰確認する。
//
// testdata が無い CI 環境では skip。docs/superpowers/specs/2026-05-11-last-played-pick-weighting-design.md
// §8 の Test 6 (テスト 7 in plan) を実装する。
func TestSL0Simulation_ProbabilityDistribution_X10(t *testing.T) {
	songdataPath := "../../../testdata/songdata.db"
	scorePath := "../../../testdata/score.db"
	md5sPath := "../../../testdata/sl0_md5s.json"
	for _, p := range []string{songdataPath, scorePath, md5sPath} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("testdata not present (%s); skipping integration sim", p)
		}
	}

	md5Bytes, err := os.ReadFile(md5sPath)
	require.NoError(t, err)
	var md5s []string
	require.NoError(t, json.Unmarshal(md5Bytes, &md5s))
	require.NotEmpty(t, md5s)

	sd, err := sql.Open("sqlite", "file:"+songdataPath+"?mode=ro")
	require.NoError(t, err)
	defer sd.Close()
	sc, err := sql.Open("sqlite", "file:"+scorePath+"?mode=ro")
	require.NoError(t, err)
	defer sc.Close()

	// md5 → sha256 → date を引いて、プレイ済み (date > 0) の集合を作る
	var dates []int64
	for _, md5 := range md5s {
		var sha string
		err := sd.QueryRow(`SELECT sha256 FROM song WHERE md5 = ? LIMIT 1`, md5).Scan(&sha)
		if err == sql.ErrNoRows {
			continue // 未所持
		}
		require.NoError(t, err)

		var date sql.NullInt64
		err = sc.QueryRow(`SELECT MAX(date) FROM score WHERE sha256 = ? AND date > 0`, sha).Scan(&date)
		if err == sql.ErrNoRows || !date.Valid || date.Int64 <= 0 {
			continue // 未プレイ / date 欠落
		}
		require.NoError(t, err)
		dates = append(dates, date.Int64)
	}
	require.Greater(t, len(dates), 100, "プレイ済み曲が想定より少ない")

	sort.Slice(dates, func(i, j int) bool { return dates[i] < dates[j] })
	oldestDate := dates[0]
	newestDate := dates[len(dates)-1]
	medianDate := dates[len(dates)/2]
	maxAge := newestDate - oldestDate
	require.Greater(t, maxAge, int64(0))

	// 重み計算: t_now = newestDate (= 最新基準) で a を計算
	x := 10.0
	w := LastPlayedWeighter{X: x}
	aOf := func(d int64) float64 {
		age := newestDate - d
		if age < 0 {
			age = 0
		}
		return float64(age) / float64(maxAge)
	}

	var total float64
	for _, d := range dates {
		total += w.Weight(aOf(d))
	}

	pOldest := w.Weight(aOf(oldestDate)) / total
	pMedian := w.Weight(aOf(medianDate)) / total
	pNewest := w.Weight(aOf(newestDate)) / total

	// ブレインストーミング時のシミュレーション値:
	// X=10 最古 1.13%, 中央値 0.66%, 最新 0.11%
	// データ更新で多少ズレるので tolerance を緩め (±0.2pt)
	const tol = 0.002
	require.InDelta(t, 0.0113, pOldest, tol, "最古曲のピック確率")
	require.InDelta(t, 0.0066, pMedian, math.Max(tol, 0.003), "中央値曲のピック確率")
	require.InDelta(t, 0.0011, pNewest, tol, "最新曲のピック確率")

	t.Logf("X=%v 全 %d 曲: 最古 %.4f%% / 中央値 %.4f%% / 最新 %.4f%%",
		x, len(dates), pOldest*100, pMedian*100, pNewest*100)
}
