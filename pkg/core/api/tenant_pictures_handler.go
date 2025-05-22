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
)

// getTenantPicture est une fonction générique pour récupérer une image du tenant
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

	// Si le fichier n'existe pas, on retourne simplement 404
	if err != nil {
		if c.Writer.Status() == http.StatusNotFound {
			c.Status(http.StatusNotFound)
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
}

// uploadTenantPicture est une fonction générique pour uploader une image du tenant
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
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Vérifier que le fichier est bien un webp
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".webp") {
		c.JSON(http.StatusBadRequest, errors.New("only webp files are allowed"))
		return
	}

	// Open the uploaded file
	fileContent, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	defer fileContent.Close()

	// Read the file content
	byteContainer, err := io.ReadAll(fileContent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Save the file with tenant-specific name
	filename := fmt.Sprintf("%s-%s.webp", tenantID, pictureType)
	if err := fileservice.SaveFile(c, byteContainer, filename); err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// Fonctions publiques qui utilisent les fonctions génériques
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
