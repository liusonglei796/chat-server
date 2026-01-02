package request

type MessageRequest struct {
	Type      int    `json:"type" binding:"required"`
	Content   string `json:"content"`
	Url       string `json:"url"`
	SendId    string `json:"send_id" binding:"required"`
	ReceiveId string `json:"receive_id" binding:"required"`
	FileType  string `json:"file_type"`
	FileName  string `json:"file_name"`
	FileSize  int    `json:"file_size"`
}
