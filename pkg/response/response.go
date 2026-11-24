package response

import (
	"encoding/json"
	"net/http"
)

// StandardResponse 统一 HTTP 响应结构
type StandardResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// JSONResponse 输出 JSON 格式的成功响应
func JSONResponse(w http.ResponseWriter, statusCode int, data interface{}, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	resp := StandardResponse{
		Success: true,
		Data:    data,
		Message: message,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// JSONError 输出 JSON 格式的错误响应
func JSONError(w http.ResponseWriter, statusCode int, errMsg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	resp := StandardResponse{
		Success: false,
		Error:   errMsg,
	}
	_ = json.NewEncoder(w).Encode(resp)
}
