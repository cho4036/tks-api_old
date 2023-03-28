package repository

import (
	"github.com/openinfradev/tks-api/pkg/httpErrors"
	"github.com/openinfradev/tks-api/pkg/log"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/openinfradev/tks-api/pkg/domain"
)

// Interfaces
type IOrganizationRepository interface {
	Create(organizationId string, name string, creator uuid.UUID, phone string, description string) (domain.Organization, error)
	Fetch() (res *[]domain.Organization, err error)
	Get(organizationId string) (res domain.Organization, err error)
	Update(organizationId string, in domain.UpdateOrganizationRequest) (domain.Organization, error)
	Delete(organizationId string) (err error)
	InitWorkflow(organizationId string, workflowId string) error
}

type OrganizationRepository struct {
	db *gorm.DB
}

func NewOrganizationRepository(db *gorm.DB) IOrganizationRepository {
	return &OrganizationRepository{
		db: db,
	}
}

// Models
type Organization struct {
	gorm.Model

	ID          string `gorm:"primarykey;type:varchar(36);not null"`
	Name        string
	Description string
	Phone       string
	WorkflowId  string
	Status      domain.OrganizationStatus
	StatusDesc  string
	Creator     uuid.UUID
}

//func (c *Organization) BeforeCreate(tx *gorm.DB) (err error) {
//	c.ID = helper.GenerateOrganizationId()
//	return nil
//}

func (r *OrganizationRepository) Create(organizationId string, name string, creator uuid.UUID, phone string,
	description string) (domain.Organization, error) {
	organization := Organization{
		ID:          organizationId,
		Name:        name,
		Creator:     creator,
		Phone:       phone,
		Description: description,
	}
	res := r.db.Create(&organization)
	if res.Error != nil {
		log.Error("error is :%s(%T)", res.Error.Error(), res.Error)
		return domain.Organization{}, res.Error
	}

	return r.reflect(organization), nil
}

func (r *OrganizationRepository) Fetch() (*[]domain.Organization, error) {
	var organizations []Organization
	var out []domain.Organization

	res := r.db.Find(&organizations)
	if res.Error != nil {
		log.Error("error is :%s(%T)", res.Error.Error(), res.Error)
		return nil, res.Error
	}
	for _, organization := range organizations {
		outOrganization := r.reflect(organization)
		out = append(out, outOrganization)
	}
	return &out, nil
}

func (r *OrganizationRepository) Get(id string) (domain.Organization, error) {
	var organization Organization
	res := r.db.First(&organization, "id = ?", id)
	if res.Error != nil {
		log.Error("error is :%s(%T)", res.Error.Error(), res.Error)
		return domain.Organization{}, res.Error
	}

	return r.reflect(organization), nil
}

func (r *OrganizationRepository) Update(organizationId string, in domain.UpdateOrganizationRequest) (domain.Organization, error) {
	var organization Organization
	res := r.db.Model(&Organization{}).
		Where("id = ?", organizationId).
		Updates(Organization{
			Description: in.Description,
			Phone:       in.Phone,
		})
	if res.Error != nil {
		log.Error("error is :%s(%T)", res.Error.Error(), res.Error)
		return domain.Organization{}, res.Error
	}
	res = r.db.Model(&Organization{}).Where("id = ?", organizationId).Find(&organization)
	if res.Error != nil {
		log.Error("error is :%s(%T)", res.Error.Error(), res.Error)
		return domain.Organization{}, res.Error
	}

	return r.reflect(organization), nil
}

func (r *OrganizationRepository) Delete(organizationId string) error {
	res := r.db.Delete(&Organization{}, "id = ?", organizationId)
	if res.Error != nil {
		log.Error("error is :%s(%T)", res.Error.Error(), res.Error)
		return res.Error
	}

	return nil
}

func (r *OrganizationRepository) InitWorkflow(organizationId string, workflowId string) error {
	res := r.db.Model(&Organization{}).
		Where("ID = ?", organizationId).
		Updates(map[string]interface{}{"Status": domain.OrganizationStatus_PENDING, "WorkflowId": workflowId})
	if res.Error != nil {
		log.Error("error is :%s(%T)", res.Error.Error(), res.Error)
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpErrors.NewNotFoundError(httpErrors.NotFound)
	}

	//if res.Error != nil || res.RowsAffected == 0 {
	//	return fmt.Errorf("nothing updated in organization with id %s", organizationId)
	//}
	
	return nil
}

func (r *OrganizationRepository) reflect(organization Organization) domain.Organization {
	return domain.Organization{
		ID:          organization.ID,
		Name:        organization.Name,
		Description: organization.Description,
		Phone:       organization.Phone,
		Status:      organization.Status.String(),
		Creator:     organization.Creator.String(),
		CreatedAt:   organization.CreatedAt,
		UpdatedAt:   organization.UpdatedAt,
	}
}
