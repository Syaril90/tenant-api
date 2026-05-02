package visitors

type CreateVisitorRequest struct {
	AccountCode           string `json:"accountCode"`
	UnitCode              string `json:"unitCode"`
	VisitorName           string `json:"visitorName"`
	Purpose               string `json:"purpose"`
	VisitDate             string `json:"visitDate"`
	ArrivalWindow         string `json:"arrivalWindow"`
	VehiclePlate          string `json:"vehiclePlate"`
	ParkingSlotsRequested int    `json:"parkingSlotsRequested"`
}

type VisitorRequestsResponse struct {
	Items []Item `json:"items"`
}

type ReplaceVisitorRequestsRequest struct {
	Items []AdminApprovalInput `json:"items"`
}

type ParkingConfigsResponse struct {
	Items []BuildingConfig `json:"items"`
}

type ReplaceParkingConfigsRequest struct {
	Items []AdminBuildingConfigInput `json:"items"`
}
