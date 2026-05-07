package usecase

import "errors"

// 公開表 / ピック / サーバ層の sentinel error。
// HTTP ハンドラは errors.Is で識別してステータスコードを決定する。
var (
	ErrPublishedTableNotFound = errors.New("公開表が見つかりません")
	ErrSourceNotFetched       = errors.New("ソース表が未取得です")
	ErrSlugInvalidFormat      = errors.New("slug の形式が不正です")
	ErrSlugReserved           = errors.New("slug は予約語です")
	ErrSlugDuplicated         = errors.New("slug は既に使われています")
	ErrInvalidPickPerLevel    = errors.New("pick_per_level は 0 以上の整数である必要があります")
	ErrInvalidRefreshMode     = errors.New("refresh_mode が不正です")
	ErrSourceTableNotFound    = errors.New("ソース表が見つかりません")
	ErrServerAlreadyRunning   = errors.New("サーバは既に起動しています")
	ErrServerNotRunning       = errors.New("サーバは起動していません")
)
