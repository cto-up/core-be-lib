package core

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/shared/auth"
	access "ctoup.com/coreapp/pkg/shared/service"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func getTenantPictureFilePath(tenantID string, pictureType string) string {
	newFilePath := fmt.Sprintf("/tenants/%s/core/pictures/%s.webp", tenantID, pictureType)
	return newFilePath
}

// getTenantPicture is a generic function to get a tenant picture
func (s *TenantHandler) getTenantPicture(c *gin.Context, pictureType string) {
	// Get tenant ID from context
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	// Try to get the tenant-specific picture
	filepath := getTenantPictureFilePath(tenantID.(string), pictureType)

	s.FileService.GetFile(c, filepath)
}

// uploadTenantPicture is a generic function to upload a tenant picture
func (s *TenantHandler) uploadTenantPicture(c *gin.Context, pictureType string) {
	// Get tenant ID from context
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	if !access.IsAdmin(c) && !access.IsSuperAdmin(c) && !access.IsCustomerAdmin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Only CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN can upload tenant pictures"})
		return
	}

	// Get the file from the request
	file, err := c.FormFile("picture")
	if err != nil {
		log.Error().Err(err).Str("tenantID", tenantID.(string)).Str("pictureType", pictureType).Msg("Failed to get file from request")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Check if the file is a webp
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".webp") {
		log.Error().Str("tenantID", tenantID.(string)).Str("pictureType", pictureType).Msg("Invalid file format. Only webp files are allowed")
		c.JSON(http.StatusBadRequest, errors.New("only webp files are allowed"))
		return
	}

	// Open the uploaded file
	fileContent, err := file.Open()
	if err != nil {
		log.Error().Err(err).Str("tenantID", tenantID.(string)).Str("pictureType", pictureType).Msg("Failed to open file")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	defer fileContent.Close()

	// Read the file content
	byteContainer, err := io.ReadAll(fileContent)
	if err != nil {
		log.Error().Err(err).Str("tenantID", tenantID.(string)).Str("pictureType", pictureType).Msg("Failed to read file")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Save the file with tenant-specific name
	filepath := getTenantPictureFilePath(tenantID.(string), pictureType)
	if err := s.FileService.SaveFile(c, byteContainer, filepath); err != nil {
		log.Error().Err(err).Str("tenantID", tenantID.(string)).Str("pictureType", pictureType).Msg("Failed to save file")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// Public functions to get tenant pictures
func (s *TenantHandler) GetTenantLogo(c *gin.Context, params core.GetTenantLogoParams) {
	s.getTenantPicture(c, "logo")
}

func (s *TenantHandler) GetTenantBackground(c *gin.Context, params core.GetTenantBackgroundParams) {
	s.getTenantPicture(c, "bg")
}

func (s *TenantHandler) GetTenantBackgroundMobile(c *gin.Context, params core.GetTenantBackgroundMobileParams) {
	s.getTenantPicture(c, "bg-mobile")
}

// Admin functions to upload tenant pictures
func (s *TenantHandler) UploadTenantLogo(c *gin.Context) {
	s.uploadTenantPicture(c, "logo")
}

func (s *TenantHandler) UploadTenantBackground(c *gin.Context) {
	s.uploadTenantPicture(c, "bg")
}

func (s *TenantHandler) UploadTenantBackgroundMobile(c *gin.Context) {
	s.uploadTenantPicture(c, "bg-mobile")
}
