package usecase

import (
	"context"
	"fmt"
	"github.com/Nerzal/gocloak/v13"
	"github.com/google/uuid"
	"github.com/openinfradev/tks-api/internal/auth/request"
	"github.com/openinfradev/tks-api/internal/helper"
	"github.com/openinfradev/tks-api/internal/keycloak"
	"github.com/openinfradev/tks-api/internal/repository"
	"github.com/openinfradev/tks-api/pkg/domain"
	"github.com/openinfradev/tks-api/pkg/httpErrors"
	"github.com/openinfradev/tks-api/pkg/log"
	"github.com/pkg/errors"
	"net/http"
)

type IUserUsecase interface {
	CreateAdmin(organizationId string) (*domain.User, error)
	DeleteAdmin(organizationId string) error
	DeleteAll(ctx context.Context, organizationId string) error
	Create(ctx context.Context, user *domain.User) (*domain.User, error)
	List(ctx context.Context, organizationId string) (*[]domain.User, error)
	GetByAccountId(ctx context.Context, accountId string, organizationId string) (*domain.User, error)
	UpdateByAccountId(ctx context.Context, accountId string, user *domain.User) (*domain.User, error)
	UpdatePasswordByAccountId(ctx context.Context, accountId string, password string, organizationId string) error
	DeleteByAccountId(ctx context.Context, accountId string, organizationId string) error
}

type UserUsecase struct {
	repo repository.IUserRepository
	kc   keycloak.IKeycloak
}

func (u *UserUsecase) DeleteAll(ctx context.Context, organizationId string) error {
	// TODO: implement me as transaction
	// TODO: clean users in keycloak

	err := u.repo.Flush(organizationId)
	if err != nil {
		return err
	}

	return nil
}

func (u *UserUsecase) DeleteAdmin(organizationId string) error {
	token, err := u.kc.LoginAdmin()
	if err != nil {
		return errors.Wrap(err, "login admin failed")
	}

	user, err := u.kc.GetUser(organizationId, "admin", token)
	if err != nil {
		return errors.Wrap(err, "get user failed")
	}

	err = u.kc.DeleteUser(organizationId, "admin", token)
	if err != nil {
		return errors.Wrap(err, "delete user failed")
	}

	userUuid, err := uuid.Parse(*user.ID)
	if err != nil {
		return errors.Wrap(err, "parse user id failed")
	}

	err = u.repo.DeleteWithUuid(userUuid)
	if err != nil {
		return errors.Wrap(err, "delete user failed")
	}

	return nil
}

func (u *UserUsecase) CreateAdmin(orgainzationId string) (*domain.User, error) {
	token, err := u.kc.LoginAdmin()
	if err != nil {
		return nil, errors.Wrap(err, "login admin failed")
	}
	user := domain.User{
		AccountId: "admin",
		Password:  "admin",
		Role: domain.Role{
			Name: "admin",
		},
		Organization: domain.Organization{
			ID: orgainzationId,
		},
		Name: "admin",
	}

	// Create user in keycloak
	groups := []string{fmt.Sprintf("%s@%s", user.Role.Name, orgainzationId)}
	err = u.kc.CreateUser(orgainzationId, &gocloak.User{
		Username: gocloak.StringP(user.AccountId),
		Credentials: &[]gocloak.CredentialRepresentation{
			{
				Type:      gocloak.StringP("password"),
				Value:     gocloak.StringP(user.Password),
				Temporary: gocloak.BoolP(false),
			},
		},
		Groups: &groups,
	}, token)
	if err != nil {
		return nil, errors.Wrap(err, "creating user in keycloak failed")
	}
	keycloakUser, err := u.kc.GetUser(user.Organization.ID, user.AccountId, token)
	if err != nil {
		return nil, errors.Wrap(err, "getting user from keycloak failed")
	}

	userUuid, err := uuid.Parse(*keycloakUser.ID)
	if err != nil {
		return nil, err
	}

	hashedPassword, err := helper.HashPassword(user.Password)
	if err != nil {
		return nil, err
	}

	roles, err := u.repo.FetchRoles()
	if err != nil {
		return nil, err
	}
	for _, role := range *roles {
		if role.Name == user.Role.Name {
			user.Role.ID = role.ID
		}
	}
	roleUuid, err := uuid.Parse(user.Role.ID)
	if err != nil {
		return nil, err
	}
	resUser, err := u.repo.CreateWithUuid(userUuid, user.AccountId, user.Name, hashedPassword, user.Email,
		user.Department, user.Description, user.Organization.ID, roleUuid)
	if err != nil {
		return nil, err
	}

	return &resUser, nil
}

func (u *UserUsecase) UpdatePasswordByAccountId(ctx context.Context, accountId string, newPassword string,
	organizationId string) error {

	token, ok := request.TokenFrom(ctx)
	if ok == false {
		return fmt.Errorf("token in the context is empty")
	}

	originUser, err := u.kc.GetUser(organizationId, accountId, token)
	if err != nil {
		return err
	}

	originUser.Credentials = &[]gocloak.CredentialRepresentation{
		{
			Type:      gocloak.StringP("password"),
			Value:     gocloak.StringP(newPassword),
			Temporary: gocloak.BoolP(false),
		},
	}

	err = u.kc.UpdateUser(organizationId, originUser, token)
	if err != nil {
		return errors.Wrap(err, "updating user in keycloak failed")
	}

	// update password in DB

	user, err := u.repo.Get(accountId, organizationId)
	if err != nil {
		return errors.Wrap(err, "getting user from repository failed")
	}
	uuid, err := uuid.Parse(user.ID)
	if err != nil {
		return errors.Wrap(err, "parsing uuid failed")
	}
	hashedPassword, err := helper.HashPassword(newPassword)
	if err != nil {
		return errors.Wrap(err, "hashing password failed")
	}

	_, err = u.repo.UpdateWithUuid(uuid, user.AccountId, user.Name, hashedPassword, user.Email,
		user.Department, user.Description)
	if err != nil {
		return errors.Wrap(err, "updating user in repository failed")
	}

	return nil
}

func (u *UserUsecase) List(ctx context.Context, organizationId string) (*[]domain.User, error) {
	users, err := u.repo.List(u.repo.OrganizationFilter(organizationId))
	if err != nil {
		return nil, err
	}

	return users, nil
}

func (u *UserUsecase) GetByAccountId(ctx context.Context, accountId string, organizationId string) (*domain.User, error) {
	users, err := u.repo.List(u.repo.OrganizationFilter(organizationId),
		u.repo.AccountIdFilter(accountId))
	if err != nil {
		return nil, err
	}

	return &(*users)[0], nil
}

func (u *UserUsecase) UpdateByAccountId(ctx context.Context, accountId string, user *domain.User) (*domain.User, error) {
	userInfo, ok := request.UserFrom(ctx)
	if ok == false {
		return nil, fmt.Errorf("user in the context is empty")
	}
	token, ok := request.TokenFrom(ctx)
	if ok == false {
		return nil, fmt.Errorf("token in the context is empty")
	}

	users, err := u.repo.List(u.repo.OrganizationFilter(userInfo.GetOrganizationId()),
		u.repo.AccountIdFilter(accountId))
	if err != nil {
		if _, code := httpErrors.ErrorResponse(err); code == http.StatusNotFound {
			return nil, httpErrors.NewNotFoundError(httpErrors.NotFound)
		}
		return nil, errors.Wrap(err, "getting users from repository failed")
	}
	if len(*users) == 0 {
		return nil, fmt.Errorf("user not found")
	} else if len(*users) > 1 {
		return nil, fmt.Errorf("multiple users found")
	}

	if user.Role.Name != (*users)[0].Role.Name {
		groups := []string{fmt.Sprintf("%s@%s", user.Role.Name, userInfo.GetOrganizationId())}
		if err := u.kc.UpdateUser(userInfo.GetOrganizationId(), &gocloak.User{
			ID:     &(*users)[0].ID,
			Groups: &groups,
		}, token); err != nil {
			log.Errorf("updating user in keycloak failed: %v", err)
			return nil, httpErrors.NewInternalServerError(err)
		}
	}

	userUuid, err := uuid.Parse((*users)[0].ID)
	if err != nil {
		return nil, err
	}

	originPassword := (*users)[0].Password

	*user, err = u.repo.UpdateWithUuid(userUuid, user.AccountId, user.Name, originPassword, user.Email,
		user.Department, user.Description)
	if err != nil {
		return nil, errors.Wrap(err, "updating user in repository failed")
	}

	return user, nil
}

func (u *UserUsecase) DeleteByAccountId(ctx context.Context, accountId string, organizationId string) error {
	user, err := u.repo.Get(accountId, organizationId)
	if err != nil {
		return err
	}

	uuid, err := uuid.Parse(user.ID)
	if err != nil {
		return err
	}
	err = u.repo.DeleteWithUuid(uuid)
	if err != nil {
		return err
	}

	// Delete user in keycloak
	token, ok := request.TokenFrom(ctx)
	if ok == false {
		return fmt.Errorf("token in the context is empty")
	}
	err = u.kc.DeleteUser(organizationId, accountId, token)
	if err != nil {
		return err
	}

	return nil
}

func (u *UserUsecase) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	// Validation check

	token, ok := request.TokenFrom(ctx)
	if ok == false {
		return nil, fmt.Errorf("token in the context is empty")
	}

	// Create user in keycloak
	groups := []string{fmt.Sprintf("%s@%s", user.Role.Name, user.Organization.ID)}
	err := u.kc.CreateUser(user.Organization.ID, &gocloak.User{
		Username: gocloak.StringP(user.AccountId),
		Credentials: &[]gocloak.CredentialRepresentation{
			{
				Type:      gocloak.StringP("password"),
				Value:     gocloak.StringP(user.Password),
				Temporary: gocloak.BoolP(false),
			},
		},
		Groups: &groups,
	}, token)
	if err != nil {
		if _, err := u.kc.GetUser(user.Organization.ID, user.AccountId, token); err == nil {
			return nil, httpErrors.NewConflictError(errors.New("user already exists"))
		}

		return nil, errors.Wrap(err, "creating user in keycloak failed")
	}
	keycloakUser, err := u.kc.GetUser(user.Organization.ID, user.AccountId, token)
	if err != nil {
		return nil, errors.Wrap(err, "getting user from keycloak failed")
	}

	userUuid, err := uuid.Parse(*keycloakUser.ID)
	if err != nil {
		return nil, err
	}

	hashedPassword, err := helper.HashPassword(user.Password)
	if err != nil {
		return nil, err
	}

	roles, err := u.repo.FetchRoles()
	if err != nil {
		return nil, err
	}
	for _, role := range *roles {
		if role.Name == user.Role.Name {
			user.Role.ID = role.ID
		}
	}
	roleUuid, err := uuid.Parse(user.Role.ID)
	if err != nil {
		return nil, err
	}

	resUser, err := u.repo.CreateWithUuid(userUuid, user.AccountId, user.Name, hashedPassword, user.Email,
		user.Department, user.Description, user.Organization.ID, roleUuid)
	if err != nil {
		return nil, err
	}

	return &resUser, nil
}

func NewUserUsecase(r repository.IUserRepository, kc keycloak.IKeycloak) IUserUsecase {
	return &UserUsecase{
		repo: r,
		kc:   kc,
	}
}
