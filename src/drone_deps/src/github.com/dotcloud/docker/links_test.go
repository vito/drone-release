package docker

import (
	"github.com/dotcloud/docker/nat"
	"github.com/dotcloud/docker/runconfig"
	"strings"
	"testing"
)

func newMockLinkContainer(id string, ip string) *Container {
	return &Container{
		Config: &runconfig.Config{},
		ID:     id,
		NetworkSettings: &NetworkSettings{
			IPAddress: ip,
		},
	}
}

func TestLinkNew(t *testing.T) {
	toID := GenerateID()
	fromID := GenerateID()

	from := newMockLinkContainer(fromID, "172.0.17.2")
	from.Config.Env = []string{}
	from.State = State{Running: true}
	ports := make(nat.PortSet)

	ports[nat.Port("6379/tcp")] = struct{}{}

	from.Config.ExposedPorts = ports

	to := newMockLinkContainer(toID, "172.0.17.3")

	link, err := NewLink(to, from, "/db/docker", nil)
	if err != nil {
		t.Fatal(err)
	}

	if link == nil {
		t.FailNow()
	}
	if link.Name != "/db/docker" {
		t.Fail()
	}
	if link.Alias() != "docker" {
		t.Fail()
	}
	if link.ParentIP != "172.0.17.3" {
		t.Fail()
	}
	if link.ChildIP != "172.0.17.2" {
		t.Fail()
	}
	for _, p := range link.Ports {
		if p != nat.Port("6379/tcp") {
			t.Fail()
		}
	}
}

func TestLinkEnv(t *testing.T) {
	toID := GenerateID()
	fromID := GenerateID()

	from := newMockLinkContainer(fromID, "172.0.17.2")
	from.Config.Env = []string{"PASSWORD=gordon"}
	from.State = State{Running: true}
	ports := make(nat.PortSet)

	ports[nat.Port("6379/tcp")] = struct{}{}

	from.Config.ExposedPorts = ports

	to := newMockLinkContainer(toID, "172.0.17.3")

	link, err := NewLink(to, from, "/db/docker", nil)
	if err != nil {
		t.Fatal(err)
	}

	rawEnv := link.ToEnv()
	env := make(map[string]string, len(rawEnv))
	for _, e := range rawEnv {
		parts := strings.Split(e, "=")
		if len(parts) != 2 {
			t.FailNow()
		}
		env[parts[0]] = parts[1]
	}
	if env["DOCKER_PORT"] != "tcp://172.0.17.2:6379" {
		t.Fatalf("Expected 172.0.17.2:6379, got %s", env["DOCKER_PORT"])
	}
	if env["DOCKER_PORT_6379_TCP"] != "tcp://172.0.17.2:6379" {
		t.Fatalf("Expected tcp://172.0.17.2:6379, got %s", env["DOCKER_PORT_6379_TCP"])
	}
	if env["DOCKER_PORT_6379_TCP_PROTO"] != "tcp" {
		t.Fatalf("Expected tcp, got %s", env["DOCKER_PORT_6379_TCP_PROTO"])
	}
	if env["DOCKER_PORT_6379_TCP_ADDR"] != "172.0.17.2" {
		t.Fatalf("Expected 172.0.17.2, got %s", env["DOCKER_PORT_6379_TCP_ADDR"])
	}
	if env["DOCKER_PORT_6379_TCP_PORT"] != "6379" {
		t.Fatalf("Expected 6379, got %s", env["DOCKER_PORT_6379_TCP_PORT"])
	}
	if env["DOCKER_NAME"] != "/db/docker" {
		t.Fatalf("Expected /db/docker, got %s", env["DOCKER_NAME"])
	}
	if env["DOCKER_ENV_PASSWORD"] != "gordon" {
		t.Fatalf("Expected gordon, got %s", env["DOCKER_ENV_PASSWORD"])
	}
}
