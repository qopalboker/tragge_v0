package server

// adminMsg holds all user-facing Persian messages for admin-bff.
var adminMsg = struct {
	// Generic
	InternalError      string
	InvalidBody        string
	InvalidForm        string
	AccessDenied       string
	AdminRequired      string
	NoFieldsToUpdate   string
	ServiceUnavailable string
	RequestTimeout     string

	// Auth
	InvalidCredentials     string
	AccountLocked          string
	TooManyLoginAttempts   string
	SystemAccountBlocked   string
	TwoFANotConfigured     string
	TwoFAUnavailable       string
	TwoFATooMany           string
	TwoFAInvalidCode       string
	TwoFATicketRequired    string
	TwoFACodeDigits        string
	InvalidOrExpiredTicket string

	// Users
	UserNotFound         string
	UserIDRequired       string
	UserAlreadyExists    string
	CannotModifyOwnRoles string
	OnlySuperAdmin       string
	RolesUpdated         string
	UserBanned           string
	UserUnbanned         string
	SessionsTerminated   string
	AdminNotFound        string
	AdminIDRequired      string

	// Contests
	ContestIDRequired      string
	ContestNotFound        string
	ContestDeleted         string
	CannotModifyState      string
	OnlyDraftDeletable     string
	ContestFull            string
	RegistrationClosed     string
	MinParticipantsNotMet  string
	ContestFinalState      string
	InvalidStateTransition string
	ParticipantNotFound    string
	ParticipantRemoved     string
	ContestUserIDRequired  string
	FreeFeeZero            string
	FreePlatformFeeZero    string

	// Wallet
	WalletNotFound      string
	WalletCharged       string
	WalletNoWallet      string
	InsufficientBalance string
	DuplicateCharge     string
	RefundFailed        string
	RefundFeeFailed     string
	WalletFrozen        string
	ChargeWalletFailed  string
	CreateWalletFailed  string

	// KYC
	KYCNotFound             string
	KYCNotReviewable        string
	KYCApproved             string
	KYCRejected             string
	KYCInfoRequested        string
	KYCBulkApproved         string
	KYCNoEligible           string
	KYCDocNotFound          string
	KYCDocLegacy            string
	KYCDocIDRequired        string
	KYCInvalidImageType     string
	KYCTooManyRejected      string
	KYCInvalidStatus        string
	KYCImageNotFound        string
	KYCStorageNotConfigured string
	InvalidRejectedField    string
	FieldMessageKeyMismatch string

	// Tickets
	TicketNotFound         string
	TicketClosed           string
	MessageRequired        string
	MessageTooLong         string
	AttachmentNotFound     string
	AttachmentUpload       string
	AttachmentDownload     string
	InvalidStatus          string
	InvalidPriority        string
	FileStorageUnavailable string
	InvalidFileType        string

	// Withdrawals
	WithdrawalNotPending       string
	WithdrawalCannotReject     string
	WithdrawalMustBeProcessing string
	WithdrawalIDRequired       string
	WithdrawalNotFound         string
	WithdrawalApproved         string
	WithdrawalRejected         string
	WithdrawalCompleted        string
	WithdrawalFailed           string
	CommentAdded               string

	// Templates & Schedules
	TemplateIDRequired      string
	TemplateNotFound        string
	TemplateCreated         string
	TemplateUpdated         string
	TemplateDeactivated     string
	TemplateAlreadyInactive string
	TemplateResetDefault    string
	TemplateKeyExists       string
	InvalidTemplateKey      string
	ScheduleIDRequired      string
	ScheduleNotFound        string
	SchedulePaused          string
	ScheduleResumed         string
	ScheduleDeactivated     string
	ScheduleAlreadyActive   string
	ScheduleAlreadyPaused   string

	// Email templates
	VersionNameRequired string
	VersionNotFound     string
	VersionDeleted      string
	CannotDeleteActive  string
	HTMLBodyRequired    string
	HTMLContentInvalid  string
	SlugRequired        string
	SlugInvalid         string
	EmailNotConfigured  string
	InvalidRequest      string
	MaxVersionsReached  string
	InvalidFontConfig   string
	FontURLNotTrusted   string

	// Symbols
	SymbolRequired     string
	SymbolNameRequired string
	SymbolNotFound     string
	SymbolExists       string
	SymbolCreated      string
	SymbolUpdated      string
	AtLeastOneSymbol   string

	// Avatars
	AvatarNotFound           string
	AvatarInUse              string
	AvatarDeleted            string
	AvatarsReordered         string
	SlugExists               string
	SlugDisplayRequired      string
	InvalidCategory          string
	ImageRequired            string
	ImageNotFound            string
	FileTooLarge             string
	InvalidImageFormat       string
	FileStorageNotConfigured string

	// Market
	MarketIngestorUnreachable string
	MarketIngestorDown        string
	SpreadNotFound            string
	SpreadUpdated             string
	DefaultSpreadUpdated      string
	AssetSpreadUpdated        string
	SpreadDeleted             string
	InvalidProvider           string
	ProviderRequired          string
	RedisUnavailable          string
	PricesFailed              string
	ResponseReadFailed        string

	// Shards
	ShardIDRequired     string
	ShardNotFound       string
	ShardActivated      string
	ShardDraining       string
	ShardActivateFailed string
	ShardDrainFailed    string

	// Tiers
	TierNotFound           string
	TierIDRequired         string
	TemplateIDRequiredTier string
	TierCreated            string
	TierUpdated            string
	TierDeactivated        string
	DuplicateEntryFee      string
	DuplicateTierEntryFee  string
	MaxTiers               string
	FreeTierEntryFee       string
	EntryFeeNonNegative    string
	CommissionRange        string
	AtLeastOneTier         string
	TiersFailed            string

	// Calendar
	CalendarNotFound   string
	CalendarUpdated    string
	CalendarDeleted    string
	InvalidDateFormat  string
	InvalidTimezone    string
	InvalidRecurrence  string
	CalendarIDRequired string

	// Financial
	DepositsFailed     string
	TransactionsFailed string
	WithdrawalsFailed  string
	EntryFeesFailed    string
	PrizesFailed       string

	// Referral
	ReferralNotFound     string
	ActivationNotPending string
	ActivationApproved   string
	ActivationRejected   string

	// Audit
	StatusUpdated   string
	InvalidFromDate string
	InvalidToDate   string

	// Tournament
	TournamentIDRequired string
	TournamentNotFound   string
	InvalidMarketType    string
	InvalidDurationType  string
	HasTiersInvalid      string
	TierCountMinInvalid  string
	IsActiveInvalid      string

	// Spread validation
	SpreadBPSMax        string
	DefaultSpreadBPSMin string
	DefaultSpreadBPSMax string
}{
	InternalError:      "خطای داخلی سرور",
	InvalidBody:        "درخواست نامعتبر است",
	InvalidForm:        "فرم ارسالی نامعتبر است",
	AccessDenied:       "دسترسی غیرمجاز",
	AdminRequired:      "دسترسی ادمین لازم است",
	NoFieldsToUpdate:   "فیلدی برای به‌روزرسانی ارسال نشده",
	ServiceUnavailable: "سرویس موقتاً در دسترس نیست",
	RequestTimeout:     "درخواست منقضی شد",

	// Auth
	InvalidCredentials:     "ایمیل یا رمز عبور اشتباه است",
	AccountLocked:          "حساب موقتاً قفل شده. لطفاً بعداً تلاش کنید.",
	TooManyLoginAttempts:   "تلاش‌های ناموفق بیش از حد مجاز. لطفاً بعداً تلاش کنید.",
	SystemAccountBlocked:   "ورود با حساب سیستمی مجاز نیست",
	TwoFANotConfigured:     "احراز هویت دو مرحله‌ای تنظیم نشده",
	TwoFAUnavailable:       "سرویس احراز هویت دو مرحله‌ای در دسترس نیست",
	TwoFATooMany:           "تلاش‌های بیش از حد مجاز. لطفاً بعداً تلاش کنید.",
	TwoFAInvalidCode:       "کد تأیید نامعتبر",
	TwoFATicketRequired:    "تیکت و کد الزامی هستند",
	TwoFACodeDigits:        "کد باید ۶ رقم باشد",
	InvalidOrExpiredTicket: "تیکت نامعتبر یا منقضی شده",

	// Users
	UserNotFound:         "کاربر یافت نشد",
	UserIDRequired:       "شناسه کاربر الزامی است",
	UserAlreadyExists:    "کاربری با این ایمیل وجود دارد",
	CannotModifyOwnRoles: "نمی‌توانید نقش خود را تغییر دهید",
	OnlySuperAdmin:       "فقط سوپر ادمین مجاز به این عملیات است",
	RolesUpdated:         "نقش‌ها با موفقیت به‌روزرسانی شد",
	UserBanned:           "کاربر مسدود شد",
	UserUnbanned:         "مسدودیت کاربر برداشته شد",
	SessionsTerminated:   "نشست‌ها با موفقیت بسته شدند",
	AdminNotFound:        "ادمین یافت نشد",
	AdminIDRequired:      "شناسه ادمین الزامی است",

	// Contests
	ContestIDRequired:      "شناسه مسابقه الزامی است",
	ContestNotFound:        "مسابقه یافت نشد",
	ContestDeleted:         "مسابقه با موفقیت حذف شد",
	CannotModifyState:      "امکان تغییر مسابقه در این وضعیت وجود ندارد",
	OnlyDraftDeletable:     "فقط مسابقات پیش‌نویس قابل حذف هستند. برای لغو از cancel استفاده کنید.",
	ContestFull:            "ظرفیت مسابقه تکمیل شده",
	RegistrationClosed:     "ثبت‌نام بسته شده",
	MinParticipantsNotMet:  "حداقل تعداد شرکت‌کنندگان تأمین نشده",
	ContestFinalState:      "مسابقه در وضعیت نهایی است",
	InvalidStateTransition: "انتقال وضعیت نامعتبر",
	ParticipantNotFound:    "شرکت‌کننده در مسابقه یافت نشد",
	ParticipantRemoved:     "شرکت‌کننده حذف شد",
	ContestUserIDRequired:  "شناسه مسابقه و کاربر الزامی است",
	FreeFeeZero:            "هزینه ورودی برای مسابقات رایگان باید صفر باشد",
	FreePlatformFeeZero:    "کارمزد پلتفرم برای مسابقات رایگان باید صفر باشد",

	// Wallet
	WalletNotFound:      "کیف پول یافت نشد",
	WalletCharged:       "کیف پول با موفقیت شارژ شد",
	WalletNoWallet:      "کاربر کیف پول ندارد",
	InsufficientBalance: "موجودی کافی نیست",
	DuplicateCharge:     "شارژ تکراری شناسایی شد",
	RefundFailed:        "خطا در بازگشت وجه",
	RefundFeeFailed:     "خطا در بازگشت کارمزد",
	WalletFrozen:        "کیف پول مسدود شده",
	ChargeWalletFailed:  "خطا در شارژ کیف پول",
	CreateWalletFailed:  "خطا در ساخت کیف پول",

	// KYC
	KYCNotFound:             "درخواست احراز هویت یافت نشد",
	KYCNotReviewable:        "درخواست احراز هویت قابل بررسی نیست",
	KYCApproved:             "احراز هویت تأیید شد",
	KYCRejected:             "احراز هویت رد شد",
	KYCInfoRequested:        "اطلاعات تکمیلی درخواست شد",
	KYCBulkApproved:         "تأیید دسته‌ای انجام شد",
	KYCNoEligible:           "درخواست واجد شرایطی یافت نشد",
	KYCDocNotFound:          "مدرک یافت نشد",
	KYCDocLegacy:            "مدرک در فرمت قدیمی ذخیره شده و دیگر قابل دسترسی نیست",
	KYCDocIDRequired:        "شناسه مدرک الزامی است",
	KYCInvalidImageType:     "نوع تصویر نامعتبر. مقادیر مجاز: front, back, selfie, selfie_with_doc",
	KYCTooManyRejected:      "تعداد فیلدهای رد شده بیش از حد مجاز (حداکثر ۱۰)",
	KYCInvalidStatus:        "فیلتر وضعیت نامعتبر",
	KYCImageNotFound:        "تصویر یافت نشد",
	KYCStorageNotConfigured: "سرویس ذخیره‌سازی تنظیم نشده",
	InvalidRejectedField:    "فیلد رد شده نامعتبر است",
	FieldMessageKeyMismatch: "کلید پیام فیلد در لیست فیلدهای رد شده نیست",

	// Tickets
	TicketNotFound:         "تیکت یافت نشد",
	TicketClosed:           "ارسال پیام به تیکت بسته شده مجاز نیست",
	MessageRequired:        "متن پیام الزامی است",
	MessageTooLong:         "متن پیام حداکثر ۵۰۰۰ کاراکتر",
	AttachmentNotFound:     "پیوست یافت نشد",
	AttachmentUpload:       "خطا در آپلود پیوست",
	AttachmentDownload:     "خطا در دانلود پیوست",
	InvalidStatus:          "وضعیت نامعتبر",
	InvalidPriority:        "اولویت نامعتبر",
	FileStorageUnavailable: "سرویس ذخیره‌سازی فایل در دسترس نیست",
	InvalidFileType:        "فایل نامعتبر است",

	// Withdrawals
	WithdrawalNotPending:       "برداشت در وضعیت انتظار نیست",
	WithdrawalCannotReject:     "امکان رد این برداشت وجود ندارد",
	WithdrawalMustBeProcessing: "برداشت باید در وضعیت پردازش باشد",
	WithdrawalIDRequired:       "شناسه برداشت الزامی است",
	WithdrawalNotFound:         "برداشت یافت نشد",
	WithdrawalApproved:         "برداشت تأیید شد",
	WithdrawalRejected:         "برداشت رد و وجه بازگردانده شد",
	WithdrawalCompleted:        "برداشت با موفقیت تکمیل شد",
	WithdrawalFailed:           "برداشت ناموفق و وجه بازگردانده شد",
	CommentAdded:               "نظر با موفقیت اضافه شد",

	// Templates & Schedules
	TemplateIDRequired:      "شناسه قالب الزامی است",
	TemplateNotFound:        "قالب یافت نشد",
	TemplateCreated:         "قالب ساخته شد",
	TemplateUpdated:         "قالب به‌روزرسانی شد",
	TemplateDeactivated:     "قالب غیرفعال شد",
	TemplateAlreadyInactive: "قالب قبلاً غیرفعال است",
	TemplateResetDefault:    "قالب به حالت پیش‌فرض بازنشانی شد",
	TemplateKeyExists:       "کلید قالب تکراری است",
	InvalidTemplateKey:      "کلید قالب نامعتبر است",
	ScheduleIDRequired:      "شناسه زمان‌بندی الزامی است",
	ScheduleNotFound:        "زمان‌بندی یافت نشد",
	SchedulePaused:          "زمان‌بندی متوقف شد",
	ScheduleResumed:         "زمان‌بندی از سر گرفته شد",
	ScheduleDeactivated:     "زمان‌بندی غیرفعال شد",
	ScheduleAlreadyActive:   "زمان‌بندی قبلاً فعال است",
	ScheduleAlreadyPaused:   "زمان‌بندی قبلاً متوقف است",

	// Email templates
	VersionNameRequired: "نام نسخه الزامی است",
	VersionNotFound:     "نسخه یافت نشد",
	VersionDeleted:      "نسخه حذف شد",
	CannotDeleteActive:  "نسخه فعال قابل حذف نیست. ابتدا نسخه دیگری فعال کنید.",
	HTMLBodyRequired:    "محتوای HTML الزامی است",
	HTMLContentInvalid:  "محتوای HTML نامعتبر است",
	SlugRequired:        "شناسه یکتا (slug) الزامی است",
	SlugInvalid:         "شناسه یکتا (slug) باید حروف کوچک، عدد و خط تیره باشد (۲ تا ۵۰ کاراکتر)",
	EmailNotConfigured:  "سرویس ایمیل تنظیم نشده",
	InvalidRequest:      "درخواست نامعتبر",
	MaxVersionsReached:  "حداکثر تعداد نسخه‌ها پر شده",
	InvalidFontConfig:   "فرمت تنظیمات فونت نامعتبر است",
	FontURLNotTrusted:   "آدرس فونت باید از منبع معتبر Google Fonts باشد",

	// Symbols
	SymbolRequired:     "نماد الزامی است",
	SymbolNameRequired: "نام نماد الزامی است",
	SymbolNotFound:     "نماد یافت نشد",
	SymbolExists:       "نماد از قبل وجود دارد",
	SymbolCreated:      "نماد ساخته شد",
	SymbolUpdated:      "نماد به‌روزرسانی شد",
	AtLeastOneSymbol:   "حداقل یک نماد الزامی است",

	// Avatars
	AvatarNotFound:           "آواتار یافت نشد",
	AvatarInUse:              "امکان حذف وجود ندارد، کاربرانی از این آواتار استفاده می‌کنند",
	AvatarDeleted:            "آواتار حذف شد",
	AvatarsReordered:         "ترتیب آواتارها تغییر کرد",
	SlugExists:               "این شناسه یکتا از قبل وجود دارد",
	SlugDisplayRequired:      "شناسه یکتا و نام نمایشی الزامی هستند",
	InvalidCategory:          "دسته‌بندی باید animal, character یا special باشد",
	ImageRequired:            "فایل تصویر الزامی است",
	ImageNotFound:            "تصویر یافت نشد",
	FileTooLarge:             "حجم فایل بیش از حد مجاز (حداکثر ۲ مگابایت)",
	InvalidImageFormat:       "فقط فرمت‌های JPG، PNG و WebP مجاز هستند",
	FileStorageNotConfigured: "سرویس ذخیره‌سازی فایل تنظیم نشده",

	// Market
	MarketIngestorUnreachable: "سرویس دریافت قیمت در دسترس نیست",
	MarketIngestorDown:        "سرویس دریافت قیمت پاسخ نمی‌دهد. ممکن است در حال راه‌اندازی مجدد باشد.",
	SpreadNotFound:            "تنظیم اسپرد یافت نشد",
	SpreadUpdated:             "اسپرد به‌روزرسانی شد",
	DefaultSpreadUpdated:      "اسپرد پیش‌فرض به‌روزرسانی شد",
	AssetSpreadUpdated:        "اسپرد کلاس دارایی به‌روزرسانی شد",
	SpreadDeleted:             "اسپرد حذف شد",
	InvalidProvider:           "ارائه‌دهنده نامعتبر است",
	ProviderRequired:          "ارائه‌دهنده الزامی است",
	RedisUnavailable:          "سرویس Redis در دسترس نیست",
	PricesFailed:              "خطا در دریافت قیمت‌ها",
	ResponseReadFailed:        "خطا در خواندن پاسخ",

	// Shards
	ShardIDRequired:     "شناسه شارد الزامی است",
	ShardNotFound:       "شارد یافت نشد",
	ShardActivated:      "شارد فعال شد",
	ShardDraining:       "شارد در حال تخلیه",
	ShardActivateFailed: "خطا در فعال‌سازی شارد",
	ShardDrainFailed:    "خطا در تخلیه شارد",

	// Tiers
	TierNotFound:           "سطح یافت نشد",
	TierIDRequired:         "شناسه سطح الزامی است",
	TemplateIDRequiredTier: "شناسه قالب الزامی است",
	TierCreated:            "سطح ساخته شد",
	TierUpdated:            "سطح به‌روزرسانی شد",
	TierDeactivated:        "سطح غیرفعال شد",
	DuplicateEntryFee:      "هزینه ورودی تکراری است",
	DuplicateTierEntryFee:  "هزینه ورودی تکراری است",
	MaxTiers:               "حداکثر ۲۰ سطح مجاز است",
	FreeTierEntryFee:       "سطح رایگان باید هزینه ورودی صفر داشته باشد",
	EntryFeeNonNegative:    "هزینه ورودی نمی‌تواند منفی باشد",
	CommissionRange:        "درصد کمیسیون باید بین ۰ تا ۵۰ باشد",
	AtLeastOneTier:         "حداقل یک سطح الزامی است",
	TiersFailed:            "خطا در عملیات سطوح",

	// Calendar
	CalendarNotFound:   "رویداد تقویم یافت نشد",
	CalendarUpdated:    "رویداد تقویم به‌روزرسانی شد",
	CalendarDeleted:    "رویداد تقویم حذف شد",
	InvalidDateFormat:  "فرمت تاریخ نامعتبر است",
	InvalidTimezone:    "منطقه زمانی نامعتبر است",
	InvalidRecurrence:  "الگوی تکرار نامعتبر است",
	CalendarIDRequired: "شناسه رویداد تقویم الزامی است",

	// Financial
	DepositsFailed:     "خطا در دریافت اطلاعات واریزها",
	TransactionsFailed: "خطا در دریافت اطلاعات تراکنش‌ها",
	WithdrawalsFailed:  "خطا در دریافت اطلاعات برداشت‌ها",
	EntryFeesFailed:    "خطا در دریافت اطلاعات ورودی‌ها",
	PrizesFailed:       "خطا در دریافت اطلاعات جوایز",

	// Referral
	ReferralNotFound:     "کد معرفی یافت نشد",
	ActivationNotPending: "درخواست فعال‌سازی در انتظار نیست",
	ActivationApproved:   "فعال‌سازی همکاری تأیید شد",
	ActivationRejected:   "فعال‌سازی همکاری رد شد",

	// Audit
	StatusUpdated:   "وضعیت به‌روزرسانی شد",
	InvalidFromDate: "فرمت تاریخ شروع نامعتبر. از RFC3339 استفاده کنید",
	InvalidToDate:   "فرمت تاریخ پایان نامعتبر. از RFC3339 استفاده کنید",

	// Tournament
	TournamentIDRequired: "شناسه تورنمنت الزامی است",
	TournamentNotFound:   "تورنمنت یافت نشد",
	InvalidMarketType:    "نوع بازار نامعتبر. مقادیر مجاز: crypto, forex",
	InvalidDurationType:  "نوع مدت زمان نامعتبر",
	HasTiersInvalid:      "مقدار has_tiers باید true یا false باشد",
	TierCountMinInvalid:  "حداقل تعداد سطوح باید عدد نامنفی باشد",
	IsActiveInvalid:      "مقدار is_active باید true یا false باشد",

	// Spread validation
	SpreadBPSMax:        "اسپرد نمی‌تواند بیش از ۱۰۰۰ (۱۰٪) باشد",
	DefaultSpreadBPSMin: "اسپرد پیش‌فرض باید حداقل ۱ باشد",
	DefaultSpreadBPSMax: "اسپرد پیش‌فرض نمی‌تواند بیش از ۱۰۰۰ (۱۰٪) باشد",
}
