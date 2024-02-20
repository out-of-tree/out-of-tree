package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReq(t *testing.T) {
	req := Req{}

	req.Command = ListRepos
	req.SetData(&Job{ID: 999, RepoName: "test"})

	bytes := req.Marshal()

	req2, err := Req{}.Unmarshal(bytes)
	assert.Nil(t, err)

	assert.Equal(t, req, req2)

	job := Job{}
	err = req2.GetData(&job)
	assert.Nil(t, err)

	assert.Equal(t, req2.Type, "*api.Job")
}

func TestResp(t *testing.T) {
	resp := Resp{}

	resp.Error = "abracadabra"
	resp.SetData(&[]Repo{{}, {}})

	bytes := resp.Marshal()

	resp2, err := Resp{}.Unmarshal(bytes)
	assert.Nil(t, err)

	resp2.Err = nil // non-marshallable

	assert.Equal(t, resp, resp2)

	var repos []Repo
	err = resp2.GetData(&repos)
	assert.Nil(t, err)

	assert.Equal(t, resp2.Type, "*[]api.Repo")
}
