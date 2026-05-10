package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// slug 正規表現: 先頭は英数字、本体は英小文字 / 数字 / ハイフン、最大 63 文字。
var slugRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// 予約 slug。HTML ビューやアプリ内部用パスとの衝突を避けるため事前禁止。
// 先頭 `_` のものは予約とみなす（バリデーション側で `_` 始まりも弾く）。
var reservedSlugs = map[string]struct{}{
	"_admin":      {},
	"_health":     {},
	"_metrics":    {},
	"_refresh":    {},
	"static":      {},
	"assets":      {},
	"favicon.ico": {},
	"robots.txt":  {},
}

// PublishedTableUseCase は公開表 CRUD と slug バリデーション/自動生成を担う。
type PublishedTableUseCase struct {
	repo    port.PublishedTableRepo
	srcRepo port.SourceTableRepo
	idGen   port.IDGenerator
	log     *slog.Logger
}

// NewPublishedTableUseCase は新しい PublishedTableUseCase を作る。
func NewPublishedTableUseCase(
	repo port.PublishedTableRepo,
	srcRepo port.SourceTableRepo,
	idGen port.IDGenerator,
	log *slog.Logger,
) *PublishedTableUseCase {
	return &PublishedTableUseCase{repo: repo, srcRepo: srcRepo, idGen: idGen, log: log}
}

// PublishedTableLevelInput は公開レベル 1 件分の入力。
type PublishedTableLevelInput struct {
	Name           string
	PerMappingPick int
	TotalPick      int
	Mappings       []PublishedTableLevelMappingInput
}

// PublishedTableLevelMappingInput は公開レベル内の 1 マッピング入力。
type PublishedTableLevelMappingInput struct {
	SourceTableID string
	SourceLevel   string
}

// CreatePublishedTableInput は Create が受け取る入力。
type CreatePublishedTableInput struct {
	Slug        string
	DisplayName string
	Symbol      string
	OwnedOnly   bool
	RefreshMode domain.RefreshMode
	Levels      []PublishedTableLevelInput
}

// UpdatePublishedTableInput は Update が受け取る入力。
type UpdatePublishedTableInput struct {
	ID          string
	Slug        string
	DisplayName string
	Symbol      string
	OwnedOnly   bool
	RefreshMode domain.RefreshMode
	SortOrder   int
	Levels      []PublishedTableLevelInput
}

// List は全公開表を返す。
func (u *PublishedTableUseCase) List(ctx context.Context) ([]domain.PublishedTable, error) {
	return u.repo.List(ctx)
}

// Get は指定 ID の公開表を返す。
func (u *PublishedTableUseCase) Get(ctx context.Context, id string) (domain.PublishedTable, error) {
	return u.repo.Get(ctx, id)
}

// Create は入力を検証して PublishedTable を作る。
func (u *PublishedTableUseCase) Create(ctx context.Context, in CreatePublishedTableInput) (string, error) {
	if err := u.validateBasic(ctx, in.Slug, "", in.RefreshMode); err != nil {
		return "", err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return "", errors.New("表示名は必須です")
	}
	id := u.idGen.New()
	levels, err := u.buildLevelsFromInput(ctx, in.Levels, id)
	if err != nil {
		return "", err
	}
	t := domain.PublishedTable{
		ID: id, Slug: in.Slug, DisplayName: in.DisplayName, Symbol: in.Symbol,
		OwnedOnly: in.OwnedOnly,
		Pick:      domain.PickConfig{RefreshMode: in.RefreshMode},
		Levels:    levels,
	}
	out, err := u.repo.Create(ctx, t)
	if err != nil {
		return "", err
	}
	u.log.Info("published table created", "id", out, "slug", in.Slug, "levels", len(levels))
	return out, nil
}

// Update は入力を検証して PublishedTable を更新する。
func (u *PublishedTableUseCase) Update(ctx context.Context, in UpdatePublishedTableInput) error {
	if in.ID == "" {
		return errors.New("ID は必須です")
	}
	if err := u.validateBasic(ctx, in.Slug, in.ID, in.RefreshMode); err != nil {
		return err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return errors.New("表示名は必須です")
	}
	levels, err := u.buildLevelsFromInput(ctx, in.Levels, in.ID)
	if err != nil {
		return err
	}
	t := domain.PublishedTable{
		ID: in.ID, Slug: in.Slug, DisplayName: in.DisplayName, Symbol: in.Symbol,
		OwnedOnly: in.OwnedOnly,
		Pick:      domain.PickConfig{RefreshMode: in.RefreshMode},
		SortOrder: in.SortOrder,
		Levels:    levels,
	}
	if err := u.repo.Update(ctx, t); err != nil {
		return err
	}
	u.log.Info("published table updated", "id", in.ID, "slug", in.Slug, "levels", len(levels))
	return nil
}

// Delete は ID で公開表を削除する。
func (u *PublishedTableUseCase) Delete(ctx context.Context, id string) error {
	if err := u.repo.Delete(ctx, id); err != nil {
		return err
	}
	u.log.Info("published table deleted", "id", id)
	return nil
}

// CreateFromSourceTable はソース表 1 件をテンプレに、各レベルを 1:1 で写像した公開表を作る。
// refresh_mode は manual 固定（後から GUI で変更可）。
func (u *PublishedTableUseCase) CreateFromSourceTable(ctx context.Context, sourceTableID, slug, displayName, symbol string) (string, error) {
	src, err := u.srcRepo.Get(ctx, sourceTableID)
	if err != nil {
		return "", ErrSourceTableNotFound
	}
	levels := make([]PublishedTableLevelInput, 0, len(src.LevelOrder))
	for _, lvl := range src.LevelOrder {
		levels = append(levels, PublishedTableLevelInput{
			Name: lvl, PerMappingPick: 0, TotalPick: 0,
			Mappings: []PublishedTableLevelMappingInput{
				{SourceTableID: sourceTableID, SourceLevel: lvl},
			},
		})
	}
	return u.Create(ctx, CreatePublishedTableInput{
		Slug: slug, DisplayName: displayName, Symbol: symbol,
		RefreshMode: domain.RefreshModeManual,
		Levels:      levels,
	})
}

// ApplyBulkPickConfig は公開表内の全レベルに同一の (m, n) を一括適用する。
func (u *PublishedTableUseCase) ApplyBulkPickConfig(ctx context.Context, id string, m, n int) error {
	if m < 0 || n < 0 {
		return ErrInvalidPickPerLevel
	}
	pub, err := u.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	for i := range pub.Levels {
		pub.Levels[i].PerMappingPick = m
		pub.Levels[i].TotalPick = n
	}
	if err := u.repo.Update(ctx, pub); err != nil {
		return err
	}
	u.log.Info("bulk pick config applied", "id", id, "m", m, "n", n)
	return nil
}

// ValidateSlug は slug の形式 / 予約語 / 重複を検査する（GUI のリアルタイム判定用）。
func (u *PublishedTableUseCase) ValidateSlug(ctx context.Context, slug string, excludeID string) error {
	if err := validateSlugFormat(slug); err != nil {
		return err
	}
	exists, err := u.repo.SlugExists(ctx, slug, excludeID)
	if err != nil {
		return err
	}
	if exists {
		return ErrSlugDuplicated
	}
	return nil
}

// SuggestSlugFromSource はソース表名（DisplayName || Name）から slug を生成する。
// 衝突時は末尾に -2, -3, ... を付与して空き番号を返す。
func (u *PublishedTableUseCase) SuggestSlugFromSource(ctx context.Context, sourceID string) (string, error) {
	src, err := u.srcRepo.Get(ctx, sourceID)
	if err != nil {
		return "", ErrSourceTableNotFound
	}
	base := slugify(firstNonEmpty(src.DisplayName, src.Name))
	if base == "" {
		base = "published"
	}
	candidate := base
	for i := 2; ; i++ {
		exists, err := u.repo.SlugExists(ctx, candidate, "")
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
		if i > 100 {
			return "", errors.New("slug 候補が見つかりません")
		}
	}
}

// buildLevelsFromInput は Input から domain 型への変換 + バリデーションを行う。
// pubID が決まっていれば Levels.PublishedTableID を埋めて返す。
// 各 Level / Mapping には新規 ID を採番する。
func (u *PublishedTableUseCase) buildLevelsFromInput(ctx context.Context, inputs []PublishedTableLevelInput, pubID string) ([]domain.PublishedTableLevel, error) {
	seenName := map[string]struct{}{}
	out := make([]domain.PublishedTableLevel, 0, len(inputs))
	for i, lin := range inputs {
		name := strings.TrimSpace(lin.Name)
		if name == "" {
			return nil, fmt.Errorf("公開レベル %d: 名前が空です", i+1)
		}
		if _, dup := seenName[name]; dup {
			return nil, ErrDuplicateLevelName
		}
		seenName[name] = struct{}{}
		if lin.PerMappingPick < 0 || lin.TotalPick < 0 {
			return nil, ErrInvalidPickPerLevel
		}
		seenMap := map[string]struct{}{}
		ms := make([]domain.PublishedTableLevelMapping, 0, len(lin.Mappings))
		for j, mp := range lin.Mappings {
			if _, err := u.srcRepo.Get(ctx, mp.SourceTableID); err != nil {
				return nil, ErrSourceTableNotFound
			}
			key := mp.SourceTableID + "\x00" + mp.SourceLevel
			if _, dup := seenMap[key]; dup {
				return nil, ErrDuplicateMapping
			}
			seenMap[key] = struct{}{}
			ms = append(ms, domain.PublishedTableLevelMapping{
				ID:            u.idGen.New(),
				SourceTableID: mp.SourceTableID,
				SourceLevel:   mp.SourceLevel,
				SortOrder:     j,
			})
		}
		lvlID := u.idGen.New()
		for k := range ms {
			ms[k].PublishedTableLevelID = lvlID
		}
		out = append(out, domain.PublishedTableLevel{
			ID:               lvlID,
			PublishedTableID: pubID,
			Name:             name,
			SortOrder:        i,
			PerMappingPick:   lin.PerMappingPick,
			TotalPick:        lin.TotalPick,
			Mappings:         ms,
		})
	}
	return out, nil
}

// validateBasic は slug / RefreshMode / 重複検査の共通部分。
func (u *PublishedTableUseCase) validateBasic(ctx context.Context, slug, excludeID string, mode domain.RefreshMode) error {
	if err := validateSlugFormat(slug); err != nil {
		return err
	}
	switch mode {
	case domain.RefreshModePerRequest, domain.RefreshModeDaily, domain.RefreshModeManual:
	default:
		return ErrInvalidRefreshMode
	}
	exists, err := u.repo.SlugExists(ctx, slug, excludeID)
	if err != nil {
		return err
	}
	if exists {
		return ErrSlugDuplicated
	}
	return nil
}

// validateSlugFormat は slug の文字種・長さ・予約語を検査する。
func validateSlugFormat(slug string) error {
	if strings.HasPrefix(slug, "_") {
		return ErrSlugReserved
	}
	if _, ok := reservedSlugs[slug]; ok {
		return ErrSlugReserved
	}
	if !slugRegexp.MatchString(slug) {
		return ErrSlugInvalidFormat
	}
	return nil
}

// slugify は文字列を kebab-case 化する。英数字以外はハイフンに置換し、連続ハイフンを 1 つにまとめ、両端を削る。
func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-")
	return out
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
