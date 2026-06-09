package invoice_comments

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/go-playground/validator/v10"
)

type InvoiceCommentHandler struct {
	service   *InvoiceCommentsService
	validator *validator.Validate
}

func NewInvoiceCommentHandler(s *InvoiceCommentsService) *InvoiceCommentHandler {
	return &InvoiceCommentHandler{
		service:   s,
		validator: validator.New(),
	}
}

// CreateComment godoc
// @Summary      Post a comment on an invoice
// @Tags         comments
// @Accept       json
// @Produce      json
// @Param        body  body      CreateCommentDTO  true  "Comment data (include token for guest access)"
// @Success      201   {object}  utils.ApiResponse
// @Failure      400   {object}  utils.ApiResponse
// @Router       /comments [post]
func (ch *InvoiceCommentHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	var dto CreateCommentDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		ch.service.log.Error("error", err.Error(), "failed to decode request body into json")
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid request body",
		})
		return
	}

	if err := ch.validator.Struct(dto); err != nil {
		ch.service.log.Error("error", err.Error(), "failed to validate request body")
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "validation failed",
		})
		return
	}
	var userIDPtr *string
	var tokenData *utils.InvoiceAccessData

	if dto.Token != "" {
		commentMeta, err := utils.VerifyInvoiceAccessToken(dto.Token, os.Getenv("INVOICE_ACCESS_SECRET"))
		if err != nil {
			ch.service.log.Error("error", err.Error(), "failed to verify invoice access token")
			utils.WriteToJson(w, http.StatusForbidden, utils.ApiResponse{
				StatusCode: http.StatusForbidden,
				Message:    "access failed, kindly contact support",
			})
			return
		}
		tokenData = commentMeta
		userId, _ := utils.GetUserIDFromContext(r.Context())
		if userId != "" {
			userIDPtr = &userId
		}
	} else {
		userID, ok := utils.GetUserIDFromContext(r.Context())
		if !ok || userID == "" {
			ch.service.log.Warn("unauthenticated request without token")
			utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "authentication required - please provide a token or sign in",
			})
			return
		}
		userIDPtr = &userID
	}

	response := ch.service.CreateComment(r.Context(), dto, userIDPtr, tokenData)
	utils.WriteToJson(w, response.StatusCode, response)
}

// GetComments godoc
// @Summary      List comments for an invoice
// @Tags         comments
// @Produce      json
// @Param        invoice_id  query  string  true   "Invoice ID"
// @Param        parent_id   query  string  false  "Parent comment ID (for replies)"
// @Param        page        query  int     false  "Page number"
// @Param        limit       query  int     false  "Items per page"
// @Param        sort_order  query  string  false  "ASC or DESC"
// @Success      200  {object}  utils.ApiResponse
// @Router       /comments [get]
func (ch *InvoiceCommentHandler) GetComments(w http.ResponseWriter, r *http.Request) {
	userID, _ := utils.GetUserIDFromContext(r.Context())
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	var query GetCommentsQuery
	query.InvoiceID = r.URL.Query().Get("invoice_id")
	query.ParentID = r.URL.Query().Get("parent_id")
	query.SortOrder = r.URL.Query().Get("sort_order")

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil {
			query.Page = page
		}
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			query.Limit = limit
		}
	}

	if query.InvoiceID == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "invoice_id is required",
		})
		return
	}

	response := ch.service.GetComments(r.Context(), query, userIDPtr)
	utils.WriteToJson(w, response.StatusCode, response)
}

// GetCommentByID godoc
// @Summary      Get comment by ID
// @Tags         comments
// @Produce      json
// @Param        id  path  string  true  "Comment ID"
// @Success      200  {object}  utils.ApiResponse
// @Router       /comments/{id} [get]
func (ch *InvoiceCommentHandler) GetCommentByID(w http.ResponseWriter, r *http.Request) {
	userID, _ := utils.GetUserIDFromContext(r.Context())
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	commentID := r.PathValue("id")
	if commentID == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "comment_id is required",
		})
		return
	}

	response := ch.service.GetCommentByID(r.Context(), commentID, userIDPtr)
	utils.WriteToJson(w, response.StatusCode, response)
}

// UpdateComment godoc
// @Summary      Edit a comment
// @Tags         comments
// @Accept       json
// @Produce      json
// @Param        id    path  string           true  "Comment ID"
// @Param        body  body  UpdateCommentDTO  true  "Updated comment text"
// @Success      200  {object}  utils.ApiResponse
// @Failure      400  {object}  utils.ApiResponse
// @Router       /comments/{id} [patch]
func (ch *InvoiceCommentHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	userID, _ := utils.GetUserIDFromContext(r.Context())
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	commentID := r.PathValue("id")

	if commentID == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "comment_id is required",
		})
		return
	}

	var dto UpdateCommentDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid request body",
			Error:      err.Error(),
		})
		return
	}

	if err := ch.validator.Struct(dto); err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "validation failed",
			Error:      err.Error(),
		})
		return
	}

	var guestEmailPtr *string
	if dto.Token != "" {
		tokenData, err := utils.VerifyInvoiceAccessToken(dto.Token, os.Getenv("INVOICE_ACCESS_SECRET"))
		if err != nil {
			utils.WriteToJson(w, http.StatusForbidden, utils.ApiResponse{
				StatusCode: http.StatusForbidden,
				Message:    "invalid access token",
			})
			return
		}
		guestEmailPtr = &tokenData.Email
	}
	response := ch.service.UpdateComment(r.Context(), commentID, dto, userIDPtr, guestEmailPtr)
	utils.WriteToJson(w, response.StatusCode, response)
}

// AddReaction godoc
// @Summary      Add emoji reaction to a comment
// @Tags         comments
// @Accept       json
// @Produce      json
// @Param        id    path  string             true  "Comment ID"
// @Param        body  body  ReactToCommentDTO  true  "Emoji and optional guest token"
// @Success      200  {object}  utils.ApiResponse
// @Router       /comments/{id}/react [post]
func (ch *InvoiceCommentHandler) AddReaction(w http.ResponseWriter, r *http.Request) {
	userID, _ := utils.GetUserIDFromContext(r.Context())
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	commentID := r.PathValue("id")
	if commentID == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "comment_id is required"})
		return
	}

	var dto ReactToCommentDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid request body"})
		return
	}
	if err := ch.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}

	response := ch.service.AddReaction(r.Context(), commentID, dto, userIDPtr)
	utils.WriteToJson(w, response.StatusCode, response)
}

// RemoveReaction godoc
// @Summary      Remove emoji reaction from a comment
// @Tags         comments
// @Produce      json
// @Param        id     path   string  true  "Comment ID"
// @Param        emoji  query  string  true  "Emoji to remove"
// @Param        email  query  string  false "Guest email"
// @Success      200  {object}  utils.ApiResponse
// @Router       /comments/{id}/react [delete]
func (ch *InvoiceCommentHandler) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	userID, _ := utils.GetUserIDFromContext(r.Context())
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	commentID := r.PathValue("id")
	emoji := r.URL.Query().Get("emoji")
	if commentID == "" || emoji == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "comment_id and emoji are required"})
		return
	}

	guestEmail := r.URL.Query().Get("email")
	var guestEmailPtr *string
	if guestEmail != "" {
		guestEmailPtr = &guestEmail
	}

	response := ch.service.RemoveReaction(r.Context(), commentID, emoji, userIDPtr, guestEmailPtr)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (ch *InvoiceCommentHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	userID, _ := utils.GetUserIDFromContext(r.Context())
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	guestEmail := r.URL.Query().Get("email")
	var guestEmailPtr *string
	if guestEmail != "" {
		guestEmailPtr = &guestEmail
	}

	commentID := r.PathValue("id")

	if commentID == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "comment_id is required",
		})
		return
	}

	response := ch.service.DeleteComment(r.Context(), commentID, userIDPtr, guestEmailPtr, false)
	utils.WriteToJson(w, response.StatusCode, response)
}
