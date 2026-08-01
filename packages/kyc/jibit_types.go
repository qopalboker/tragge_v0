package kyc

// ShahkarRequest is the request body for the Shahkar (phone+nationalCode match) API.
type ShahkarRequest struct {
	MobileNumber string `json:"mobileNumber"`
	NationalCode string `json:"nationalCode"`
}

// ShahkarResult is the response from the Shahkar API.
type ShahkarResult struct {
	Matched        bool   `json:"matched"`
	MobileOperator string `json:"mobileOperator"`
	TransactionID  string `json:"transactionId"`
}

// IdentityInfoRequest is the request body for the identity info inquiry API.
type IdentityInfoRequest struct {
	NationalCode string `json:"nationalCode"`
	BirthDate    string `json:"birthDate"` // Jalali date, e.g. "1370/01/01"
}

// IdentityInfoResult is the response from the identity info inquiry API.
type IdentityInfoResult struct {
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	FatherName   string `json:"fatherName"`
	Alive        bool   `json:"alive"`
	NationalCode string `json:"nationalCode"`
}

// CardToNIDRequest is the request body for the card-to-nationalCode match API.
type CardToNIDRequest struct {
	CardNumber   string `json:"cardNumber"`
	NationalCode string `json:"nationalCode"`
}

// CardToNIDResult is the response from the card-to-nationalCode match API.
type CardToNIDResult struct {
	Matched bool `json:"matched"`
}

// FaceVerificationResult is the response from the biometric face verification API.
type FaceVerificationResult struct {
	Matched        bool    `json:"matched"`
	MatchScore     float64 `json:"matchScore"`
	LivenessScore  float64 `json:"livenessScore"`
	LivenessResult string  `json:"livenessResult"` // "LIVE" or "FAKE"
}

// NationalCardOCRResult is the response from the national card OCR API.
type NationalCardOCRResult struct {
	NationalCode string `json:"nationalCode"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	BirthDate    string `json:"birthDate"`
	ExpiryDate   string `json:"expiryDate"`
	SerialNumber string `json:"serialNumber"`
}

// JibitTokenRequest is the request body for obtaining a Jibit access token.
type JibitTokenRequest struct {
	APIKey    string `json:"apiKey"`
	SecretKey string `json:"secretKey"`
}

// JibitTokenResponse is the response from the Jibit token API.
type JibitTokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int64  `json:"expiresIn"` // seconds
}

// JibitRefreshRequest is the request body for refreshing a Jibit access token.
type JibitRefreshRequest struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// JibitErrorResponse represents an error response from the Jibit API.
type JibitErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
