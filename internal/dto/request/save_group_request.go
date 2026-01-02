package request

type SaveGroupRequest struct {
	Uuid    string `json:"uuid" binding:"required"`
	OwnerId string `json:"owner_id" binding:"required"`
	Name    string `json:"name" binding:"required"`
	Notice  string `json:"notice"`
	AddMode int    `json:"add_mode"`
	Avatar  string `json:"avatar"`
}
