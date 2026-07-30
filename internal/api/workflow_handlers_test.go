package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/usecase"
	"go.uber.org/zap"
)

const validWorkflowDefinition = `{
	"nodes":[
		{"id":"start","type":"trigger","name":"Start","config":{}},
		{"id":"work","type":"task","name":"Work","config":{"queue":"default"}}
	],
	"edges":[{"id":"start-work","from":"start","to":"work","condition":null}]
}`

type workflowRepositoryStub struct {
	workflows map[uuid.UUID]*domain.Workflow
	versions  map[uuid.UUID][]*domain.WorkflowVersion
}

func newWorkflowRepositoryStub() *workflowRepositoryStub {
	return &workflowRepositoryStub{
		workflows: make(map[uuid.UUID]*domain.Workflow),
		versions:  make(map[uuid.UUID][]*domain.WorkflowVersion),
	}
}

func (repository *workflowRepositoryStub) CreateWorkflow(_ context.Context, workflow *domain.Workflow) error {
	for _, stored := range repository.workflows {
		if stored.OrgID == workflow.OrgID && stored.Slug == workflow.Slug && stored.DeletedAt == nil {
			return domain.ErrWorkflowSlugConflict
		}
	}
	repository.workflows[workflow.ID] = cloneWorkflow(workflow)
	return nil
}

func (repository *workflowRepositoryStub) GetWorkflowByID(_ context.Context, orgID, id uuid.UUID) (*domain.Workflow, error) {
	workflow, exists := repository.workflows[id]
	if !exists || workflow.OrgID != orgID || workflow.DeletedAt != nil {
		return nil, domain.ErrWorkflowNotFound
	}
	return cloneWorkflow(workflow), nil
}

func (repository *workflowRepositoryStub) GetWorkflowBySlug(_ context.Context, orgID uuid.UUID, slug string) (*domain.Workflow, error) {
	for _, workflow := range repository.workflows {
		if workflow.OrgID == orgID && workflow.Slug == slug && workflow.DeletedAt == nil {
			return cloneWorkflow(workflow), nil
		}
	}
	return nil, domain.ErrWorkflowNotFound
}

func (repository *workflowRepositoryStub) ListWorkflows(_ context.Context, orgID uuid.UUID, filter domain.WorkflowFilter) (*domain.WorkflowPage, error) {
	items := make([]*domain.Workflow, 0)
	for _, workflow := range repository.workflows {
		if workflow.OrgID == orgID && workflow.DeletedAt == nil && (filter.Status == "" || workflow.Status == filter.Status) {
			items = append(items, cloneWorkflow(workflow))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return &domain.WorkflowPage{Workflows: items}, nil
}

func (repository *workflowRepositoryStub) UpdateWorkflow(_ context.Context, workflow *domain.Workflow) error {
	stored, exists := repository.workflows[workflow.ID]
	if !exists || stored.OrgID != workflow.OrgID || stored.DeletedAt != nil {
		return domain.ErrWorkflowNotFound
	}
	for id, candidate := range repository.workflows {
		if id != workflow.ID && candidate.OrgID == workflow.OrgID && candidate.Slug == workflow.Slug && candidate.DeletedAt == nil {
			return domain.ErrWorkflowSlugConflict
		}
	}
	repository.workflows[workflow.ID] = cloneWorkflow(workflow)
	return nil
}

func (repository *workflowRepositoryStub) SoftDeleteWorkflow(_ context.Context, orgID, id uuid.UUID, now time.Time) error {
	workflow, exists := repository.workflows[id]
	if !exists || workflow.OrgID != orgID || workflow.DeletedAt != nil {
		return domain.ErrWorkflowNotFound
	}
	workflow.DeletedAt = &now
	workflow.UpdatedAt = now
	return nil
}

func (repository *workflowRepositoryStub) PublishWorkflow(
	_ context.Context,
	expected *domain.Workflow,
	publishedAt time.Time,
) (*domain.WorkflowVersion, error) {
	workflow, exists := repository.workflows[expected.ID]
	if !exists || workflow.OrgID != expected.OrgID || workflow.DeletedAt != nil {
		return nil, domain.ErrWorkflowNotFound
	}
	if !workflow.UpdatedAt.Equal(expected.UpdatedAt) {
		return nil, domain.ErrWorkflowPublishConflict
	}
	version := &domain.WorkflowVersion{
		ID: uuid.New(), OrgID: workflow.OrgID, WorkflowID: workflow.ID,
		Version: len(repository.versions[workflow.ID]) + 1,
		Name:    workflow.Name, Slug: workflow.Slug, Description: workflow.Description,
		Definition: workflow.Definition, Status: domain.WorkflowVersionStatusPublished,
		PublishedAt: publishedAt, CreatedAt: publishedAt,
	}
	repository.versions[workflow.ID] = append(repository.versions[workflow.ID], cloneWorkflowVersion(version))
	workflow.Status = domain.WorkflowStatusPublished
	workflow.UpdatedAt = publishedAt
	return cloneWorkflowVersion(version), nil
}

func (repository *workflowRepositoryStub) CreateWorkflowVersion(_ context.Context, version *domain.WorkflowVersion) error {
	repository.versions[version.WorkflowID] = append(repository.versions[version.WorkflowID], cloneWorkflowVersion(version))
	return nil
}

func (repository *workflowRepositoryStub) GetWorkflowVersion(
	_ context.Context,
	orgID, workflowID uuid.UUID,
	version int,
) (*domain.WorkflowVersion, error) {
	workflow, exists := repository.workflows[workflowID]
	if !exists || workflow.OrgID != orgID || workflow.DeletedAt != nil {
		return nil, domain.ErrWorkflowVersionNotFound
	}
	for _, candidate := range repository.versions[workflowID] {
		if candidate.Version == version {
			return cloneWorkflowVersion(candidate), nil
		}
	}
	return nil, domain.ErrWorkflowVersionNotFound
}

func (repository *workflowRepositoryStub) ListWorkflowVersions(
	_ context.Context,
	orgID, workflowID uuid.UUID,
) (*domain.WorkflowVersionPage, error) {
	workflow, exists := repository.workflows[workflowID]
	if !exists || workflow.OrgID != orgID || workflow.DeletedAt != nil {
		return nil, domain.ErrWorkflowNotFound
	}
	items := make([]domain.WorkflowVersionSummary, 0, len(repository.versions[workflowID]))
	for index := len(repository.versions[workflowID]) - 1; index >= 0; index-- {
		version := repository.versions[workflowID][index]
		items = append(items, domain.WorkflowVersionSummary{
			Version: version.Version, VersionID: version.ID, Status: version.Status,
			PublishedAt: version.PublishedAt, Name: version.Name, Slug: version.Slug,
		})
	}
	return &domain.WorkflowVersionPage{Versions: items}, nil
}

func (repository *workflowRepositoryStub) GetLatestWorkflowVersion(
	ctx context.Context,
	orgID, workflowID uuid.UUID,
) (*domain.WorkflowVersion, error) {
	versions := repository.versions[workflowID]
	if len(versions) == 0 {
		return nil, domain.ErrWorkflowVersionNotFound
	}
	return repository.GetWorkflowVersion(ctx, orgID, workflowID, versions[len(versions)-1].Version)
}

func cloneWorkflow(workflow *domain.Workflow) *domain.Workflow {
	encoded, _ := json.Marshal(workflow)
	var cloned domain.Workflow
	_ = json.Unmarshal(encoded, &cloned)
	cloned.DeletedAt = workflow.DeletedAt
	return &cloned
}

func cloneWorkflowVersion(version *domain.WorkflowVersion) *domain.WorkflowVersion {
	encoded, _ := json.Marshal(version)
	var cloned domain.WorkflowVersion
	_ = json.Unmarshal(encoded, &cloned)
	return &cloned
}

func TestWorkflowCRUDIsTenantScoped(t *testing.T) {
	repository := newWorkflowRepositoryStub()
	orgOne, orgTwo := uuid.New(), uuid.New()
	orgOneRouter := testWorkflowRouter(repository, orgOne)
	orgTwoRouter := testWorkflowRouter(repository, orgTwo)

	createdResponse := performWorkflowRequest(orgOneRouter, http.MethodPost, "/v1/workflows", `{
		"name":"  Daily Orders  ","definition":`+validWorkflowDefinition+`
	}`)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created domain.Workflow
	decodeWorkflowResponse(t, createdResponse, &created)
	if created.Name != "Daily Orders" || created.Slug != "daily-orders" || created.Status != domain.WorkflowStatusDraft {
		t.Fatalf("created workflow = %+v", created)
	}

	providedResponse := performWorkflowRequest(orgOneRouter, http.MethodPost, "/v1/workflows", `{
		"name":"Provided","slug":"provided-slug","definition":`+validWorkflowDefinition+`
	}`)
	if providedResponse.Code != http.StatusCreated {
		t.Fatalf("provided slug create = %d: %s", providedResponse.Code, providedResponse.Body.String())
	}

	duplicateResponse := performWorkflowRequest(orgOneRouter, http.MethodPost, "/v1/workflows", `{
		"name":"Duplicate","slug":"daily-orders","definition":`+validWorkflowDefinition+`
	}`)
	if duplicateResponse.Code != http.StatusConflict {
		t.Fatalf("same-org duplicate status = %d: %s", duplicateResponse.Code, duplicateResponse.Body.String())
	}
	crossOrgCreate := performWorkflowRequest(orgTwoRouter, http.MethodPost, "/v1/workflows", `{
		"name":"Allowed","slug":"daily-orders","definition":`+validWorkflowDefinition+`
	}`)
	if crossOrgCreate.Code != http.StatusCreated {
		t.Fatalf("cross-org same slug status = %d: %s", crossOrgCreate.Code, crossOrgCreate.Body.String())
	}

	listOne := performWorkflowRequest(orgOneRouter, http.MethodGet, "/v1/workflows", "")
	var pageOne domain.WorkflowPage
	decodeWorkflowResponse(t, listOne, &pageOne)
	if listOne.Code != http.StatusOK || len(pageOne.Workflows) != 2 {
		t.Fatalf("org-one list = %d %+v", listOne.Code, pageOne)
	}
	listTwo := performWorkflowRequest(orgTwoRouter, http.MethodGet, "/v1/workflows", "")
	var pageTwo domain.WorkflowPage
	decodeWorkflowResponse(t, listTwo, &pageTwo)
	if listTwo.Code != http.StatusOK || len(pageTwo.Workflows) != 1 {
		t.Fatalf("org-two list = %d %+v", listTwo.Code, pageTwo)
	}

	workflowPath := "/v1/workflows/" + created.ID.String()
	for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		body := ""
		if method == http.MethodPatch {
			body = `{"name":"Forbidden"}`
		}
		response := performWorkflowRequest(orgTwoRouter, method, workflowPath, body)
		if response.Code != http.StatusNotFound {
			t.Fatalf("cross-org %s status = %d: %s", method, response.Code, response.Body.String())
		}
	}

	updateResponse := performWorkflowRequest(orgOneRouter, http.MethodPatch, workflowPath, `{
		"name":"Updated orders","slug":"updated-orders","description":"  draft flow  "
	}`)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", updateResponse.Code, updateResponse.Body.String())
	}
	var updated domain.Workflow
	decodeWorkflowResponse(t, updateResponse, &updated)
	if updated.Name != "Updated orders" || updated.Slug != "updated-orders" || updated.Description == nil || *updated.Description != "draft flow" {
		t.Fatalf("updated workflow = %+v", updated)
	}

	deleteResponse := performWorkflowRequest(orgOneRouter, http.MethodDelete, workflowPath, "")
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if response := performWorkflowRequest(orgOneRouter, http.MethodGet, workflowPath, ""); response.Code != http.StatusNotFound {
		t.Fatalf("soft-deleted get status = %d: %s", response.Code, response.Body.String())
	}
	listAfterDelete := performWorkflowRequest(orgOneRouter, http.MethodGet, "/v1/workflows", "")
	var afterDelete domain.WorkflowPage
	decodeWorkflowResponse(t, listAfterDelete, &afterDelete)
	if len(afterDelete.Workflows) != 1 {
		t.Fatalf("soft-deleted list = %+v", afterDelete)
	}
}

func TestWorkflowCreateValidation(t *testing.T) {
	router := testWorkflowRouter(newWorkflowRepositoryStub(), uuid.New())
	tests := []struct {
		name string
		body string
	}{
		{name: "missing name", body: `{"definition":` + validWorkflowDefinition + `}`},
		{name: "invalid slug", body: `{"name":"Test","slug":"Not Safe","definition":` + validWorkflowDefinition + `}`},
		{name: "invalid definition shape", body: `{"name":"Test","definition":[]}`},
		{name: "duplicate node ids", body: `{"name":"Test","definition":{"nodes":[{"id":"same","type":"task","name":"One","config":{}},{"id":"same","type":"delay","name":"Two","config":{}}],"edges":[]}}`},
		{name: "duplicate edge ids", body: `{"name":"Test","definition":{"nodes":[{"id":"a","type":"task","name":"A","config":{}},{"id":"b","type":"task","name":"B","config":{}}],"edges":[{"id":"same","from":"a","to":"b","condition":null},{"id":"same","from":"b","to":"a","condition":null}]}}`},
		{name: "unknown edge node", body: `{"name":"Test","definition":{"nodes":[{"id":"a","type":"task","name":"A","config":{}}],"edges":[{"id":"bad","from":"a","to":"missing","condition":null}]}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performWorkflowRequest(router, http.MethodPost, "/v1/workflows", test.body)
			if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"validation_failed"`)) {
				t.Fatalf("status/body = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestWorkflowValidationAndPublishingLifecycle(t *testing.T) {
	repository := newWorkflowRepositoryStub()
	orgOne, orgTwo := uuid.New(), uuid.New()
	orgOneRouter := testWorkflowRouter(repository, orgOne)
	orgTwoRouter := testWorkflowRouter(repository, orgTwo)

	createdResponse := performWorkflowRequest(orgOneRouter, http.MethodPost, "/v1/workflows", `{
		"name":"Publish me","definition":`+validWorkflowDefinition+`
	}`)
	var created domain.Workflow
	decodeWorkflowResponse(t, createdResponse, &created)
	path := "/v1/workflows/" + created.ID.String()

	validateResponse := performWorkflowRequest(orgOneRouter, http.MethodPost, path+"/validate", "")
	var validation domain.WorkflowValidationResult
	decodeWorkflowResponse(t, validateResponse, &validation)
	if validateResponse.Code != http.StatusOK || !validation.Valid || len(validation.Errors) != 0 {
		t.Fatalf("validation = %d %+v", validateResponse.Code, validation)
	}

	firstPublish := performWorkflowRequest(orgOneRouter, http.MethodPost, path+"/publish", "")
	var first domain.WorkflowPublishResult
	decodeWorkflowResponse(t, firstPublish, &first)
	if firstPublish.Code != http.StatusCreated || first.Version != 1 || first.Status != domain.WorkflowVersionStatusPublished {
		t.Fatalf("first publish = %d %+v", firstPublish.Code, first)
	}

	update := performWorkflowRequest(orgOneRouter, http.MethodPatch, path, `{
		"name":"Publish me again",
		"definition":{
			"nodes":[
				{"id":"start","type":"trigger","name":"Start","config":{}},
				{"id":"work","type":"webhook","name":"Changed","config":{"url":"https://example.com"}}
			],
			"edges":[{"id":"start-work","from":"start","to":"work","condition":null}]
		}
	}`)
	var updated domain.Workflow
	decodeWorkflowResponse(t, update, &updated)
	if update.Code != http.StatusOK || updated.Status != domain.WorkflowStatusDraft {
		t.Fatalf("published workflow update = %d %+v", update.Code, updated)
	}

	secondPublish := performWorkflowRequest(orgOneRouter, http.MethodPost, path+"/publish", "")
	var second domain.WorkflowPublishResult
	decodeWorkflowResponse(t, secondPublish, &second)
	if secondPublish.Code != http.StatusCreated || second.Version != 2 {
		t.Fatalf("second publish = %d %+v", secondPublish.Code, second)
	}

	firstDetail := performWorkflowRequest(orgOneRouter, http.MethodGet, path+"/versions/1", "")
	var firstVersion domain.WorkflowVersion
	decodeWorkflowResponse(t, firstDetail, &firstVersion)
	if firstVersion.Name != "Publish me" || firstVersion.Definition.Nodes[1].Type != domain.WorkflowNodeTask {
		t.Fatalf("version one snapshot mutated: %+v", firstVersion)
	}
	list := performWorkflowRequest(orgOneRouter, http.MethodGet, path+"/versions", "")
	var page domain.WorkflowVersionPage
	decodeWorkflowResponse(t, list, &page)
	if list.Code != http.StatusOK || len(page.Versions) != 2 || page.Versions[0].Version != 2 {
		t.Fatalf("versions list = %d %+v", list.Code, page)
	}

	for _, route := range []string{path + "/publish", path + "/versions", path + "/versions/1"} {
		method := http.MethodGet
		if route == path+"/publish" {
			method = http.MethodPost
		}
		response := performWorkflowRequest(orgTwoRouter, method, route, "")
		if response.Code != http.StatusNotFound {
			t.Fatalf("cross-org %s = %d: %s", route, response.Code, response.Body.String())
		}
	}
}

func TestWorkflowCannotPublishInvalidGraph(t *testing.T) {
	repository := newWorkflowRepositoryStub()
	orgID := uuid.New()
	now := time.Now().UTC()
	workflow := &domain.Workflow{
		ID: uuid.New(), OrgID: orgID, Name: "Invalid", Slug: "invalid",
		Status: domain.WorkflowStatusDraft,
		Definition: domain.WorkflowDefinition{
			Nodes: []domain.WorkflowNode{
				{ID: "task", Type: domain.WorkflowNodeTask, Name: "Task", Config: map[string]any{}},
			},
			Edges: []domain.WorkflowEdge{},
		},
		CreatedAt: now, UpdatedAt: now,
	}
	repository.workflows[workflow.ID] = workflow
	router := testWorkflowRouter(repository, orgID)
	path := "/v1/workflows/" + workflow.ID.String()

	validateResponse := performWorkflowRequest(router, http.MethodPost, path+"/validate", "")
	var validation domain.WorkflowValidationResult
	decodeWorkflowResponse(t, validateResponse, &validation)
	if validateResponse.Code != http.StatusOK || validation.Valid || len(validation.Errors) == 0 {
		t.Fatalf("invalid validation response = %d %+v", validateResponse.Code, validation)
	}
	publishResponse := performWorkflowRequest(router, http.MethodPost, path+"/publish", "")
	if publishResponse.Code != http.StatusBadRequest ||
		!bytes.Contains(publishResponse.Body.Bytes(), []byte(`"code":"workflow_validation_failed"`)) {
		t.Fatalf("invalid publish = %d %s", publishResponse.Code, publishResponse.Body.String())
	}

	deletedAt := now.Add(time.Second)
	workflow.DeletedAt = &deletedAt
	if response := performWorkflowRequest(router, http.MethodPost, path+"/publish", ""); response.Code != http.StatusNotFound {
		t.Fatalf("deleted publish = %d %s", response.Code, response.Body.String())
	}
}

func TestWorkflowRoutesRequireAuthentication(t *testing.T) {
	router := testWorkflowRouter(newWorkflowRepositoryStub(), uuid.New())
	request := httptest.NewRequest(http.MethodGet, "/v1/workflows", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func testWorkflowRouter(repository *workflowRepositoryStub, orgID uuid.UUID) http.Handler {
	service := usecase.NewWorkflowService(repository)
	handler := NewHandler(&serviceStub{}, zap.NewNop()).WithWorkflowService(service)
	return NewRouter(handler, allowAuth(orgID), nil, zap.NewNop())
}

func performWorkflowRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer valid")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeWorkflowResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
}
