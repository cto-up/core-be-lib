package core

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"ctoup.com/coreapp/api/helpers"
	fileservice "ctoup.com/coreapp/pkg/shared/fileservice"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// getTenantPicture is a generic function to get a tenant picture
func (s *TenantHandler) getTenantPicture(c *gin.Context, pictureType string) {
	// Get tenant ID from context
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	// Try to get the tenant-specific picture
	filename := fmt.Sprintf("%s-%s.webp", tenantID, pictureType)
	err := fileservice.GetFile(c, os.Getenv("FILE_FOLDER_URL"), filename)

	// If the tenant-specific picture does not exist, return 404
	if err != nil {
		if c.Writer.Status() == http.StatusNotFound {
			c.Status(http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("tenantID", tenantID.(string)).Str("pictureType", pictureType).Msg("Failed to get tenant picture")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
}

// uploadTenantPicture is a generic function to upload a tenant picture
func (s *TenantHandler) uploadTenantPicture(c *gin.Context, pictureType string) {
	// Get tenant ID from context
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
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
	filename := fmt.Sprintf("%s-%s.webp", tenantID, pictureType)
	if err := fileservice.SaveFile(c, byteContainer, filename); err != nil {
		log.Error().Err(err).Str("tenantID", tenantID.(string)).Str("pictureType", pictureType).Msg("Failed to save file")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// Public functions to get tenant pictures
func (s *TenantHandler) GetTenantLogo(c *gin.Context) {
	s.getTenantPicture(c, "logo")
}

func (s *TenantHandler) GetTenantBackground(c *gin.Context) {
	s.getTenantPicture(c, "bg")
}

func (s *TenantHandler) GetTenantBackgroundMobile(c *gin.Context) {
	s.getTenantPicture(c, "bg-mobile")
}

func (s *TenantHandler) UploadTenantLogo(c *gin.Context) {
	s.uploadTenantPicture(c, "logo")
}

func (s *TenantHandler) UploadTenantBackground(c *gin.Context) {
	s.uploadTenantPicture(c, "bg")
}

func (s *TenantHandler) UploadTenantBackgroundMobile(c *gin.Context) {
	s.uploadTenantPicture(c, "bg-mobile")
}
