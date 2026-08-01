package server

// tradeMsg holds all user-facing Persian messages for trade-bff.
var tradeMsg = struct {
	// Generic
	InternalError string
	DatabaseError string

	// Auth / WebSocket
	AuthRequired           string
	AuthRequiredWithHint   string
	InvalidOrExpiredToken  string
	InvalidOrExpiredTicket string
	InvalidTicketData      string
	TicketContestMismatch  string
	ContestIDRequired      string

	// Contest state
	ContestNotFound   string
	ContestNotRunning string
	ContestNotStarted string
	ContestEnded      string
	NotParticipant    string

	// Trading
	UserNotFound           string
	OrderNotFound          string
	OrderNotYours          string
	OrderSubmitFailed      string
	OrderCancelSubmitted   string
	CannotCancelOrder      string
	CancelSubmitFailed     string
	ClosePositionSubmitted string
	ClosePositionFailed    string
	TPSLUpdated            string
	TPSLUpdateFailed       string

	// Chart
	SymbolRequired       string
	SymbolInvalid        string
	ResolutionRequired   string
	ResolutionInvalid    string
	FromRequired         string
	FromInvalid          string
	ToRequired           string
	ToInvalid            string
	FromMustBeLessThanTo string

	// Leaderboard
	LeaderboardNotAvailable string
	LeaderboardFailed       string
	UserIDRequired          string
	UserPositionNotFound    string

	// Drawings
	SymbolParamRequired  string
	InvalidJSON          string
	DrawingsPayloadLarge string
	DrawingsLoadFailed   string
	DrawingsSaveFailed   string
	DrawingsSaved        string
	DrawingsDeleted      string

	// Tournament
	TournamentNotFound string

	// Shard
	ShardInfoUnavailable string

	// Circuits
	CircuitsReset string
}{
	InternalError: "خطای داخلی سرور",
	DatabaseError: "خطای دیتابیس",

	// Auth / WebSocket
	AuthRequired:           "احراز هویت الزامی است",
	AuthRequiredWithHint:   "احراز هویت الزامی است",
	InvalidOrExpiredToken:  "توکن نامعتبر یا منقضی شده",
	InvalidOrExpiredTicket: "تیکت نامعتبر یا منقضی شده",
	InvalidTicketData:      "اطلاعات تیکت نامعتبر است",
	TicketContestMismatch:  "تیکت با مسابقه تطابق ندارد",
	ContestIDRequired:      "شناسه مسابقه الزامی است",

	// Contest state
	ContestNotFound:   "مسابقه یافت نشد",
	ContestNotRunning: "مسابقه در حال اجرا نیست",
	ContestNotStarted: "مسابقه هنوز شروع نشده",
	ContestEnded:      "مسابقه پایان یافته",
	NotParticipant:    "شما در این مسابقه شرکت نکرده‌اید",

	// Trading
	UserNotFound:           "کاربر یافت نشد",
	OrderNotFound:          "سفارش یافت نشد",
	OrderNotYours:          "این سفارش متعلق به شما نیست",
	OrderSubmitFailed:      "خطا در ارسال سفارش",
	OrderCancelSubmitted:   "درخواست لغو سفارش ثبت شد",
	CannotCancelOrder:      "امکان لغو این سفارش وجود ندارد",
	CancelSubmitFailed:     "خطا در ارسال درخواست لغو",
	ClosePositionSubmitted: "درخواست بستن پوزیشن ثبت شد",
	ClosePositionFailed:    "خطا در ارسال درخواست بستن پوزیشن",
	TPSLUpdated:            "حد سود/ضرر به‌روزرسانی شد",
	TPSLUpdateFailed:       "خطا در به‌روزرسانی حد سود/ضرر",

	// Chart
	SymbolRequired:       "پارامتر نماد الزامی است",
	SymbolInvalid:        "فرمت نماد نامعتبر است",
	ResolutionRequired:   "پارامتر تایم‌فریم الزامی است",
	ResolutionInvalid:    "تایم‌فریم نامعتبر. مقادیر مجاز: 1, 5, 15, 30, 60, 240, D, W, M",
	FromRequired:         "پارامتر شروع الزامی است",
	FromInvalid:          "پارامتر شروع نامعتبر. باید Unix timestamp ثانیه‌ای باشد",
	ToRequired:           "پارامتر پایان الزامی است",
	ToInvalid:            "پارامتر پایان نامعتبر. باید Unix timestamp ثانیه‌ای باشد",
	FromMustBeLessThanTo: "زمان شروع باید قبل از زمان پایان باشد",

	// Leaderboard
	LeaderboardNotAvailable: "جدول رده‌بندی در دسترس نیست",
	LeaderboardFailed:       "خطا در دریافت جدول رده‌بندی",
	UserIDRequired:          "شناسه کاربر الزامی است",
	UserPositionNotFound:    "جایگاه کاربر یافت نشد",

	// Drawings
	SymbolParamRequired:  "پارامتر نماد الزامی است",
	InvalidJSON:          "فرمت JSON نامعتبر است",
	DrawingsPayloadLarge: "حجم ترسیمات بیش از حد مجاز است",
	DrawingsLoadFailed:   "خطا در بارگذاری ترسیمات",
	DrawingsSaveFailed:   "خطا در ذخیره ترسیمات",
	DrawingsSaved:        "ذخیره شد",
	DrawingsDeleted:      "حذف شد",

	// Tournament
	TournamentNotFound: "تورنمنت یافت نشد",

	// Shard
	ShardInfoUnavailable: "اطلاعات شارد در دسترس نیست",

	// Circuits
	CircuitsReset: "تمام circuit breaker ها ریست شدند",
}
