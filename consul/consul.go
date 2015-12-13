package consul

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"os"
    "encoding/json"
	"github.com/gliderlabs/registrator/bridge"
	consulapi "github.com/hashicorp/consul/api"
)

const DefaultInterval = "10s"
var Hostname string
func init() {
	bridge.Register(new(Factory), "consul")
	Hostname, _ = os.Hostname()
}

func (r *ConsulAdapter) interpolateService(script string, service *bridge.Service) string {
	withIp := strings.Replace(script, "$SERVICE_IP", service.Origin.HostIP, -1)
	withPort := strings.Replace(withIp, "$SERVICE_PORT", service.Origin.HostPort, -1)
	return withPort
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consulapi.DefaultConfig()
	if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consulapi.NewClient(config)
	if err != nil {
		log.Fatal("consul: ", uri.Scheme)
	}
	return &ConsulAdapter{client: client}
}

type ConsulAdapter struct {
	client *consulapi.Client
}

// Ping will try to connect to consul by attempting to retrieve the current leader.
func (r *ConsulAdapter) Ping() error {
	status := r.client.Status()
	leader, err := status.Leader()
	if err != nil {
		return err
	}
	log.Println("consul: current leader ", leader)

	return nil
}

func (r *ConsulAdapter) Register(service *bridge.Service) error {
	agentService := new(consulapi.AgentService)
	agentService.ID = service.ID
	agentService.Service = service.Name
	agentService.Port = service.Port
	agentService.Tags = service.Tags
	agentService.Address = service.IP
	
	registration := new(consulapi.CatalogRegistration)
	registration.Node = Hostname
	registration.Address = service.Origin.HostIP
	registration.Datacenter =  service.Attrs["region"];
    registration.Service = agentService;
    registration.Check = nil

    writeOptions := new(consulapi.WriteOptions)
    writeOptions.Datacenter =  service.Attrs["region"];

	out, _ := json.Marshal(registration)

    log.Println("REGISTERING :",string(out))
	 _ , res := r.client.Catalog().Register(registration,writeOptions);

	return res
}

func (r *ConsulAdapter) buildCheck(service *bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	if path := service.Attrs["check_http"]; path != "" {
		check.HTTP = fmt.Sprintf("http://%s:%d%s", service.IP, service.Port, path)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
	} else if cmd := service.Attrs["check_cmd"]; cmd != "" {
		check.Script = fmt.Sprintf("check-cmd %s %s %s", service.Origin.ContainerID[:12], service.Origin.ExposedPort, cmd)
	} else if script := service.Attrs["check_script"]; script != "" {
		check.Script = r.interpolateService(script, service)
	} else if ttl := service.Attrs["check_ttl"]; ttl != "" {
		check.TTL = ttl
	} else {
		return nil
	}
	if check.Script != "" || check.HTTP != "" {
		if interval := service.Attrs["check_interval"]; interval != "" {
			check.Interval = interval
		} else {
			check.Interval = DefaultInterval
		}
	}
	return check
}

func (r *ConsulAdapter) Deregister(service *bridge.Service) error {
    deRegistration := new(consulapi.CatalogDeregistration)
	deRegistration.Node = Hostname
	deRegistration.Address = ""
	deRegistration.Datacenter =  service.Attrs["region"];
    deRegistration.ServiceID =  service.ID;
    deRegistration.CheckID = ""


    writeOptions := new(consulapi.WriteOptions)
    writeOptions.Datacenter =  service.Attrs["region"];

	out, _ := json.Marshal(deRegistration)

    log.Println("DE-REGISTERING :",string(out))
    
	 _ , res := r.client.Catalog().Deregister(deRegistration,writeOptions);


	return res
}

func (r *ConsulAdapter) Refresh(service *bridge.Service) error {
	return nil
}

func (r *ConsulAdapter) Services() ([]*bridge.Service, error) {
	services, err := r.client.Agent().Services()
	if err != nil {
		return []*bridge.Service{}, err
	}
	out := make([]*bridge.Service, len(services))
	i := 0
	for _, v := range services {
		s := &bridge.Service{
			ID:   v.ID,
			Name: v.Service,
			Port: v.Port,
			Tags: v.Tags,
			IP:   v.Address,
		}
		out[i] = s
		i++
	}
	return out, nil
}
