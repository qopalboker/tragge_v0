package server

// msg holds all user-facing Persian messages.
// Backend returns these directly; frontend displays data.error / data.message as-is.
var msg = struct {
	// Generic
	InternalError string
	InvalidBody   string
	InvalidForm   string

	// Auth
	InvalidCredentials    string
	CaptchaFailed         string
	AccountLocked         string
	TooManyLoginAttempts  string
	TooManyAuthAttempts   string
	SystemAccountBlocked  string
	InvalidRefreshToken   string
	RefreshFailed         string
	PasswordRequired      string
	InvalidPassword       string
	PasswordChanged       string
	PasswordResetSuccess  string
	InvalidOrExpiredToken string

	// Rate limits
	TooManyRequests        string
	TooManyContestJoins    string
	TooManyPasswordChanges string
	TooManyTickets         string
	TooManyMessages        string

	// Email verification
	EmailAlreadyVerified      string
	EmailVerifiedSuccess      string
	VerificationCodeSent      string
	VerificationCodeInvalid   string
	VerificationCodeRequired  string
	VerificationTooMany       string
	VerificationNoValidCode   string
	VerificationCodeExhausted string
	VerificationWrongCode     string // uses fmt.Sprintf
	EmailServiceUnavailable   string

	// 2FA
	TwoFAAlreadyEnabled     string
	TwoFANotConfigured      string
	TwoFASetupNotInitiated  string
	TwoFAServiceUnavailable string
	TwoFAInvalidCode        string
	TwoFATooManyAttempts    string
	TwoFAEnabledSuccess     string
	TwoFADisabledSuccess    string
	TwoFATicketRequired     string
	TwoFACodeDigits         string

	// OAuth
	OAuthNotConfigured      string
	OAuthServiceUnavailable string
	OAuthAccountExists      string
	OAuthGoogleFailed       string
	OAuthInvalidCode        string
	OAuthUserInfoFailed     string
	OAuthEmailNotVerified   string
	OAuthInvalidState       string

	// Profile
	UserNotFound           string
	PhoneAlreadyInUse      string
	UniqueViolation        string
	NoFieldsToUpdate       string
	AvatarIDRequired       string
	AvatarIDInvalid        string
	FileTooLarge           string
	InvalidFileType        string
	FileStorageUnavailable string

	// Contests
	ContestIDRequired        string
	ContestNotFound          string
	ContestNotOpen           string
	ContestFull              string
	InsufficientBalance      string
	WalletFrozen             string
	WalletNotFound           string
	AlreadyParticipant       string
	NotParticipant           string
	CannotLeaveRunning       string
	CannotLeaveOpenPositions string
	LeftContestSuccess       string
	SystemCannotJoinPaid     string
	TradeHistoryNotReady     string
	UserNotParticipated      string

	// Tickets
	SubjectRequired          string
	SubjectTooLong           string
	InvalidCategory          string
	MessageRequired          string
	MessageTooLong           string
	TicketNotFound           string
	TicketClosed             string
	AttachmentNotFound       string
	AttachmentUploadFailed   string
	AttachmentDownloadFailed string
	InvalidAttachmentFile    string
	TicketAlreadyClosed      string

	// Sessions
	SessionNotAvailable  string
	SessionNotFound      string
	SessionIDRequired    string
	CannotDeleteCurrent  string
	SessionNotYours      string
	SessionDeleted       string
	AllSessionsDeleted   string
	OtherSessionsDeleted string
	LoggedOut            string
	NoCurrentSession     string

	// Notifications
	NotificationIDRequired string
	NotificationIDInvalid  string
	NotificationNotFound   string
	NotificationNotYours   string

	// KYC
	KYCRateLimit          string
	KYCAlreadyInProgress  string
	KYCServiceUnavailable string
	FormParseFailed       string
	SelfieRequired        string
	FrontImageRequired    string
	SaveFrontFailed       string
	SaveSelfieFailed      string
	SaveSelfieDocFailed   string
	SaveBackFailed        string
	ValidationFailed      string

	// Tournaments
	TournamentIDRequired string
	TournamentNotFound   string
	InvalidMarketType    string
	InvalidDurationType  string
	InvalidStatus        string
	InvalidSortBy        string
	InvalidCursor        string
	InvalidDateFormat    string
	DateRangeTooLarge    string

	// Referral
	ReferralCodeRequired  string
	ReferralCodeNotFound  string
	ActivationAlreadyDone string
	ActivationSubmitted   string

	// Wallet
	UserIDRequired      string
	InvalidUserIDFormat string

	// Phone OTP
	InvalidPhone                     string
	OTPSent                          string
	OTPCodeInvalid                   string
	OTPCodeWrong                     string
	SMSServiceUnavailable            string
	OTPPleaseWait                    string
	OTPSendFailed                    string
	OTPVerifyFailed                  string
	OTPServiceTemporarilyUnavailable string
	OTPPasswordWeak                  string

	// Password Reset (OTP-based)
	PasswordResetCodeSent        string
	PasswordResetCodeInvalid     string
	PasswordResetCodeExpired     string
	PasswordResetTooManyAttempts string
	PasswordResetSessionExpired  string
	PasswordsMismatch            string
	PasswordSameAsOld            string
	IdentifierRequired           string
}{
	// Generic
	InternalError: "خطای داخلی سرور",
	InvalidBody:   "درخواست نامعتبر است",
	InvalidForm:   "فرم ارسالی نامعتبر است",

	// Auth
	InvalidCredentials:    "ایمیل یا رمز عبور اشتباه است",
	CaptchaFailed:         "تأیید کپچا انجام نشد",
	AccountLocked:         "حساب به‌دلیل تلاش‌های ناموفق متعدد موقتاً قفل شده. لطفاً بعداً تلاش کنید.",
	TooManyLoginAttempts:  "تعداد تلاش‌های ناموفق بیش از حد مجاز. لطفاً بعداً تلاش کنید.",
	TooManyAuthAttempts:   "تعداد درخواست‌های احراز هویت بیش از حد مجاز. لطفاً کمی صبر کنید.",
	SystemAccountBlocked:  "ورود با حساب سیستمی مجاز نیست",
	InvalidRefreshToken:   "توکن منقضی یا نامعتبر است",
	RefreshFailed:         "بازسازی نشست ناموفق بود",
	PasswordRequired:      "رمز عبور الزامی است",
	InvalidPassword:       "رمز عبور نادرست است",
	PasswordChanged:       "رمز عبور با موفقیت تغییر کرد",
	PasswordResetSuccess:  "رمز عبور با موفقیت بازنشانی شد. لطفاً با رمز جدید وارد شوید.",
	InvalidOrExpiredToken: "لینک نامعتبر یا منقضی شده است",

	// Rate limits
	TooManyRequests:        "تعداد درخواست‌ها بیش از حد مجاز. لطفاً کمی صبر کنید.",
	TooManyContestJoins:    "تعداد درخواست‌های عضویت بیش از حد مجاز. لطفاً کمی صبر کنید.",
	TooManyPasswordChanges: "تعداد تلاش‌های تغییر رمز بیش از حد مجاز. لطفاً بعداً تلاش کنید.",
	TooManyTickets:         "حداکثر ۵ تیکت در ساعت مجاز است",
	TooManyMessages:        "لطفاً بین ارسال پیام‌ها کمی صبر کنید",

	// Email verification
	EmailAlreadyVerified:      "ایمیل شما قبلاً تأیید شده است.",
	EmailVerifiedSuccess:      "ایمیل با موفقیت تأیید شد!",
	VerificationCodeSent:      "کد تأیید ارسال شد. لطفاً صندوق ورودی خود را بررسی کنید.",
	VerificationCodeInvalid:   "کد تأیید باید دقیقاً ۶ رقم باشد.",
	VerificationCodeRequired:  "کد تأیید الزامی است",
	VerificationTooMany:       "تعداد درخواست‌های تأیید بیش از حد مجاز. لطفاً کمی صبر کنید.",
	VerificationNoValidCode:   "کد تأیید معتبری یافت نشد. لطفاً کد جدید درخواست کنید.",
	VerificationCodeExhausted: "تعداد تلاش‌ها به پایان رسید. لطفاً کد جدید درخواست کنید.",
	VerificationWrongCode:     "کد نادرست است. %d تلاش باقیمانده.",
	EmailServiceUnavailable:   "سرویس ایمیل در دسترس نیست",

	// 2FA
	TwoFAAlreadyEnabled:     "احراز هویت دو مرحله‌ای قبلاً فعال شده",
	TwoFANotConfigured:      "احراز هویت دو مرحله‌ای تنظیم نشده",
	TwoFASetupNotInitiated:  "ابتدا تنظیم احراز هویت دو مرحله‌ای را آغاز کنید",
	TwoFAServiceUnavailable: "سرویس احراز هویت دو مرحله‌ای موقتاً در دسترس نیست",
	TwoFAInvalidCode:        "کد تأیید نامعتبر است",
	TwoFATooManyAttempts:    "تعداد تلاش‌ها بیش از حد مجاز. لطفاً بعداً تلاش کنید.",
	TwoFAEnabledSuccess:     "احراز هویت دو مرحله‌ای فعال شد. کدهای پشتیبان را در جای امنی ذخیره کنید.",
	TwoFADisabledSuccess:    "احراز هویت دو مرحله‌ای غیرفعال شد",
	TwoFATicketRequired:     "تیکت و کد الزامی هستند",
	TwoFACodeDigits:         "کد باید ۶ رقم باشد",

	// OAuth
	OAuthNotConfigured:      "ورود با گوگل فعال نیست",
	OAuthServiceUnavailable: "سرویس ورود با گوگل موقتاً در دسترس نیست",
	OAuthAccountExists:      "این حساب گوگل قبلاً به کاربر دیگری متصل شده",
	OAuthGoogleFailed:       "خطا در ارتباط با گوگل",
	OAuthInvalidCode:        "کد احراز هویت نامعتبر است",
	OAuthUserInfoFailed:     "خطا در دریافت اطلاعات از گوگل",
	OAuthEmailNotVerified:   "ایمیل گوگل تأیید نشده است",
	OAuthInvalidState:       "پارامتر نامعتبر یا منقضی شده",

	// Profile
	UserNotFound:           "کاربر یافت نشد",
	PhoneAlreadyInUse:      "این شماره تلفن قبلاً استفاده شده",
	UniqueViolation:        "اطلاعات تکراری است",
	NoFieldsToUpdate:       "فیلدی برای به‌روزرسانی وجود ندارد",
	AvatarIDRequired:       "شناسه آواتار الزامی است",
	AvatarIDInvalid:        "شناسه آواتار نامعتبر است",
	FileTooLarge:           "حجم فایل بیش از حد مجاز (حداکثر ۲ مگابایت)",
	InvalidFileType:        "فرمت فایل نامعتبر. فقط JPG، PNG و WebP مجاز هستند",
	FileStorageUnavailable: "سرویس ذخیره‌سازی فایل در دسترس نیست",

	// Contests
	ContestIDRequired:        "شناسه مسابقه الزامی است",
	ContestNotFound:          "مسابقه یافت نشد",
	ContestNotOpen:           "مسابقه برای ثبت‌نام باز نیست",
	ContestFull:              "ظرفیت مسابقه تکمیل شده",
	InsufficientBalance:      "موجودی کیف پول کافی نیست",
	WalletFrozen:             "کیف پول مسدود شده",
	WalletNotFound:           "کیف پول یافت نشد",
	AlreadyParticipant:       "شما قبلاً در این مسابقه شرکت کرده‌اید",
	NotParticipant:           "شما در این مسابقه شرکت نکرده‌اید",
	CannotLeaveRunning:       "خروج از مسابقه فقط در مرحله ثبت‌نام مجاز است",
	CannotLeaveOpenPositions: "ابتدا تمام معاملات باز را ببندید",
	LeftContestSuccess:       "با موفقیت از مسابقه خارج شدید",
	SystemCannotJoinPaid:     "حساب‌های سیستمی نمی‌توانند در مسابقات پولی شرکت کنند",
	TradeHistoryNotReady:     "تاریخچه معاملات پس از پایان مسابقه در دسترس است",
	UserNotParticipated:      "کاربر در این مسابقه شرکت نکرده",

	// Tickets
	SubjectRequired:          "عنوان تیکت الزامی است",
	SubjectTooLong:           "عنوان تیکت حداکثر ۲۰۰ کاراکتر",
	InvalidCategory:          "دسته‌بندی نامعتبر است",
	MessageRequired:          "متن پیام الزامی است",
	MessageTooLong:           "متن پیام حداکثر ۵۰۰۰ کاراکتر",
	TicketNotFound:           "تیکت یافت نشد",
	TicketClosed:             "ارسال پیام به تیکت بسته شده مجاز نیست",
	AttachmentNotFound:       "پیوست یافت نشد",
	AttachmentUploadFailed:   "خطا در آپلود پیوست",
	AttachmentDownloadFailed: "خطا در دانلود پیوست",
	InvalidAttachmentFile:    "فایل نامعتبر است",
	TicketAlreadyClosed:      "تیکت یافت نشد یا قبلاً بسته شده",

	// Sessions
	SessionNotAvailable:  "مدیریت نشست در دسترس نیست",
	SessionNotFound:      "نشست یافت نشد",
	SessionIDRequired:    "شناسه نشست الزامی است",
	CannotDeleteCurrent:  "نشست فعلی قابل حذف نیست. از خروج استفاده کنید.",
	SessionNotYours:      "این نشست متعلق به شما نیست",
	SessionDeleted:       "نشست با موفقیت حذف شد",
	AllSessionsDeleted:   "تمام نشست‌ها حذف شدند",
	OtherSessionsDeleted: "سایر نشست‌ها حذف شدند",
	LoggedOut:            "با موفقیت خارج شدید",
	NoCurrentSession:     "نشست فعلی موجود نیست",

	// Notifications
	NotificationIDRequired: "شناسه اعلان الزامی است",
	NotificationIDInvalid:  "شناسه اعلان نامعتبر است",
	NotificationNotFound:   "اعلان یافت نشد",
	NotificationNotYours:   "این اعلان متعلق به شما نیست",

	// KYC
	KYCRateLimit:          "حداکثر ۳ درخواست احراز هویت در ۲۴ ساعت. لطفاً بعداً تلاش کنید.",
	KYCAlreadyInProgress:  "درخواست احراز هویت در حال بررسی است",
	KYCServiceUnavailable: "سرویس احراز هویت تنظیم نشده",
	FormParseFailed:       "خطا در پردازش فرم",
	SelfieRequired:        "تصویر سلفی الزامی است",
	FrontImageRequired:    "تصویر روی مدرک الزامی است",
	SaveFrontFailed:       "خطا در ذخیره تصویر مدرک",
	SaveSelfieFailed:      "خطا در ذخیره تصویر سلفی",
	SaveSelfieDocFailed:   "خطا در ذخیره سلفی با مدرک",
	SaveBackFailed:        "خطا در ذخیره تصویر پشت مدرک",
	ValidationFailed:      "اطلاعات ارسالی نامعتبر است",

	// Tournaments
	TournamentIDRequired: "شناسه تورنمنت الزامی است",
	TournamentNotFound:   "تورنمنت یافت نشد",
	InvalidMarketType:    "نوع بازار نامعتبر است",
	InvalidDurationType:  "نوع مدت زمان نامعتبر است",
	InvalidStatus:        "وضعیت نامعتبر است",
	InvalidSortBy:        "ترتیب مرتب‌سازی نامعتبر است",
	InvalidCursor:        "نشانگر صفحه‌بندی نامعتبر است",
	InvalidDateFormat:    "فرمت تاریخ نامعتبر. از YYYY-MM-DD استفاده کنید",
	DateRangeTooLarge:    "بازه تاریخ حداکثر ۳۰ روز مجاز است",

	// Referral
	ReferralCodeRequired:  "کد معرفی الزامی است",
	ReferralCodeNotFound:  "کد معرفی یافت نشد",
	ActivationAlreadyDone: "درخواست فعال‌سازی قبلاً ثبت شده",
	ActivationSubmitted:   "درخواست فعال‌سازی ثبت شد",

	// Wallet/User
	UserIDRequired:      "شناسه کاربر الزامی است",
	InvalidUserIDFormat: "فرمت شناسه کاربر نامعتبر است",

	// Phone OTP
	InvalidPhone:                     "شماره موبایل نامعتبر است",
	OTPSent:                          "کد تأیید ارسال شد",
	OTPCodeInvalid:                   "کد تأیید باید دقیقاً ۶ رقم باشد",
	OTPCodeWrong:                     "کد تأیید نادرست است",
	SMSServiceUnavailable:            "سرویس پیامک در دسترس نیست",
	OTPPleaseWait:                    "لطفاً کمی صبر کنید و دوباره تلاش کنید",
	OTPSendFailed:                    "خطا در ارسال کد تأیید، لطفاً دوباره تلاش کنید",
	OTPVerifyFailed:                  "خطا در بررسی کد تأیید، لطفاً دوباره تلاش کنید",
	OTPServiceTemporarilyUnavailable: "سرویس پیامک موقتاً در دسترس نیست، لطفاً دقایقی بعد تلاش کنید",
	OTPPasswordWeak:                  "رمز عبور باید حداقل ۸ کاراکتر و شامل حروف بزرگ، کوچک، اعداد و نمادها باشد",

	// Password Reset (OTP-based)
	PasswordResetCodeSent:        "اگر حساب کاربری با این مشخصات وجود داشته باشد، کد بازیابی ارسال شد",
	PasswordResetCodeInvalid:     "کد وارد شده نادرست است",
	PasswordResetCodeExpired:     "کد منقضی شده. لطفاً کد جدید درخواست کنید",
	PasswordResetTooManyAttempts: "تعداد تلاش بیش از حد مجاز. لطفاً بعداً تلاش کنید",
	PasswordResetSessionExpired:  "نشست بازیابی رمز منقضی شده. لطفاً دوباره تلاش کنید",
	PasswordsMismatch:            "رمز عبور و تکرار آن مطابقت ندارند",
	PasswordSameAsOld:            "رمز جدید نباید مشابه رمز فعلی باشد",
	IdentifierRequired:           "لطفاً نام کاربری، شماره یا ایمیل را وارد کنید",
}
