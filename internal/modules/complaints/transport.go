package complaints

type ComplaintListResponse struct {
	Items []Item `json:"items"`
}

type UpdateComplaintStatusRequest struct {
	Status  string `json:"status"`
	Comment string `json:"comment"`
}
