package loadbalancer

import "testing"

func TestMemberUpdateRequestIncludesClearsAndEmptyTags(t *testing.T) {
	name := "web-01"
	weight := 2
	adminStateUp := false
	backup := true
	tags := []string{}

	body, err := memberUpdateRequest{
		Name:              &name,
		Weight:            &weight,
		AdminStateUp:      &adminStateUp,
		Backup:            &backup,
		MonitorAddressSet: true,
		MonitorPortSet:    true,
		Tags:              &tags,
	}.ToMemberUpdateMap()
	if err != nil {
		t.Fatalf("ToMemberUpdateMap error = %v", err)
	}

	memberBody, ok := body["member"].(map[string]any)
	if !ok {
		t.Fatalf("member body type = %T, want map[string]any", body["member"])
	}
	if memberBody["name"] != "web-01" {
		t.Fatalf("name = %#v, want web-01", memberBody["name"])
	}
	if memberBody["weight"] != 2 {
		t.Fatalf("weight = %#v, want 2", memberBody["weight"])
	}
	if memberBody["admin_state_up"] != false {
		t.Fatalf("admin_state_up = %#v, want false", memberBody["admin_state_up"])
	}
	if memberBody["backup"] != true {
		t.Fatalf("backup = %#v, want true", memberBody["backup"])
	}
	if _, ok := memberBody["monitor_address"]; !ok {
		t.Fatal("expected monitor_address key")
	}
	if memberBody["monitor_address"] != nil {
		t.Fatalf("monitor_address = %#v, want nil", memberBody["monitor_address"])
	}
	if _, ok := memberBody["monitor_port"]; !ok {
		t.Fatal("expected monitor_port key")
	}
	if memberBody["monitor_port"] != nil {
		t.Fatalf("monitor_port = %#v, want nil", memberBody["monitor_port"])
	}
	tagValues, ok := memberBody["tags"].([]string)
	if !ok {
		t.Fatalf("tags type = %T, want []string", memberBody["tags"])
	}
	if len(tagValues) != 0 {
		t.Fatalf("len(tags) = %d, want 0", len(tagValues))
	}
}
