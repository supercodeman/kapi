package errcode

const (
	// 通用错误
	ErrBadRequest    = 40000
	ErrUnauthorized  = 40100
	ErrForbidden     = 40300
	ErrNotFound      = 40400
	ErrInternal      = 50000

	// 认证相关 401xx
	ErrInvalidToken  = 40101
	ErrTokenExpired  = 40102
	ErrMissingToken  = 40103

	// 参数校验 400xx
	ErrInvalidParam  = 40001
	ErrMissingParam  = 40002

	// 业务错误 420xx
	ErrUserExists    = 42001
	ErrUserNotFound  = 42002
	ErrWrongPassword = 42003
	ErrBillNotFound  = 42004
	ErrBudgetNotFound = 42005
	ErrAssetNotFound  = 42006
)

var messages = map[int]string{
	ErrBadRequest:    "bad request",
	ErrUnauthorized:  "unauthorized",
	ErrForbidden:     "forbidden",
	ErrNotFound:      "not found",
	ErrInternal:      "internal server error",
	ErrInvalidToken:  "invalid token",
	ErrTokenExpired:  "token expired",
	ErrMissingToken:  "missing token",
	ErrInvalidParam:  "invalid parameter",
	ErrMissingParam:  "missing parameter",
	ErrUserExists:    "user already exists",
	ErrUserNotFound:  "user not found",
	ErrWrongPassword: "wrong password",
	ErrBillNotFound:  "bill not found",
	ErrBudgetNotFound: "budget not found",
	ErrAssetNotFound:  "asset not found",
}

func Message(code int) string {
	if msg, ok := messages[code]; ok {
		return msg
	}
	return "unknown error"
}
