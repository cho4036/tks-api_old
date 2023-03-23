package usecase

import (
	"fmt"

	"github.com/openinfradev/tks-api/internal/keycloak"

	"github.com/google/uuid"
	"github.com/openinfradev/tks-api/internal/repository"
	argowf "github.com/openinfradev/tks-api/pkg/argo-client"
	"github.com/openinfradev/tks-api/pkg/domain"
	"github.com/openinfradev/tks-api/pkg/httpErrors"
	"github.com/openinfradev/tks-api/pkg/log"
)

type IOrganizationUsecase interface {
	Fetch() ([]domain.Organization, error)
	Get(organizationId string) (domain.Organization, error)
	Create(domain.Organization, string) (organizationId string, err error)
	Delete(organizationId string, accessToken string) error
	Update(organizationId string, in domain.UpdateOrganizationRequest) (err error)
}

type OrganizationUsecase struct {
	repo repository.IOrganizationRepository
	argo argowf.ArgoClient
	kc   keycloak.IKeycloak
}

func NewOrganizationUsecase(r repository.IOrganizationRepository, argoClient argowf.ArgoClient, kc keycloak.IKeycloak) IOrganizationUsecase {
	return &OrganizationUsecase{
		repo: r,
		argo: argoClient,
		kc:   kc,
	}
}

func (u *OrganizationUsecase) Fetch() (out []domain.Organization, err error) {
	organizations, err := u.repo.Fetch()
	if err != nil {
		return nil, err
	}
	return organizations, nil
}

func (u *OrganizationUsecase) Create(in domain.Organization, accessToken string) (organizationId string, err error) {
	// Create realm in keycloak
	if organizationId, err = u.kc.CreateRealm(in.Name, domain.Organization{}, accessToken); err != nil {
		return "", err
	}

	creator := uuid.Nil
	if in.Creator != "" {
		creator, err = uuid.Parse(in.Creator)
		if err != nil {
			return "", err
		}
	}
	organizationId, err = u.repo.Create(in.Name, creator, in.Description)
	if err != nil {
		return "", err
	}
	log.Info("newly created Organization ID:", organizationId)

	workflowId, err := u.argo.SumbitWorkflowFromWftpl(
		"tks-create-contract-repo",
		argowf.SubmitOptions{
			Parameters: []string{
				"contract_id=" + organizationId,
			},
		})
	if err != nil {
		log.Error("failed to submit argo workflow template. err : ", err)
		return "", fmt.Errorf("Failed to call argo workflow : %s", err)
	}
	log.Info("submited workflow :", workflowId)

	if err := u.repo.InitWorkflow(organizationId, workflowId); err != nil {
		return "", fmt.Errorf("Failed to initialize organization status to 'CREATING'. err : %s", err)
	}

	return organizationId, nil
}

func (u *OrganizationUsecase) Get(organizationId string) (res domain.Organization, err error) {
	res, err = u.repo.Get(organizationId)
	if err != nil {
		return domain.Organization{}, httpErrors.NewNotFoundError(err)
	}
	return res, nil
}

func (u *OrganizationUsecase) Delete(organizationId string, accessToken string) (err error) {
	_, err = u.Get(organizationId)
	if err != nil {
		return err
	}

	// Delete realm in keycloak
	if err := u.kc.DeleteRealm(organizationId, accessToken); err != nil {
		return err
	}

	// [TODO] validation
	// cluster 나 appgroup 등이 삭제 되었는지 확인
	err = u.repo.Delete(organizationId)
	if err != nil {
		return err
	}

	return nil
}

func (u *OrganizationUsecase) Update(organizationId string, in domain.UpdateOrganizationRequest) (err error) {
	_, err = u.Get(organizationId)
	if err != nil {
		return httpErrors.NewNotFoundError(err)
	}

	err = u.repo.Update(organizationId, in)
	if err != nil {
		return err
	}
	return nil
}
