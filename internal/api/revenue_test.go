package api

import "testing"

func TestASaleClosesItsLeadAndFeedsTheRevenueFigures(t *testing.T) {
	h, _, _ := teamServer(t)
	_, lead := call(t, h, "POST", "/v1/leads", `{"name":"Asha Menon","email":"asha@example.com","company":"Menon Textiles"}`, token)
	leadID := lead["id"].(string)

	// A monthly plan, a yearly plan (a twelfth counts each month) and a one-time sale that never counts toward MRR.
	for _, body := range []string{
		`{"lead_id":"` + leadID + `","customer":"Asha Menon","billing":"monthly","amount":10000,"plan":"business"}`,
		`{"customer":"Kiran Shah","billing":"yearly","amount":120000,"plan":"larger"}`,
		`{"customer":"Tara Bose","billing":"one_time","amount":250000}`,
	} {
		if code, _ := call(t, h, "POST", "/v1/sales", body, token); code != 201 {
			t.Fatalf("record sale: %d", code)
		}
	}

	_, got := call(t, h, "GET", "/v1/leads/"+leadID, "", token)
	if st := got["lead"].(map[string]any)["status"]; st != "won" {
		t.Fatalf("lead status = %v, want won", st)
	}
	if got["sale"] == nil {
		t.Fatal("lead detail should carry its sale")
	}

	_, an := call(t, h, "GET", "/v1/analytics?months=6", "", token)
	if an["mrr"].(float64) != 20000 || an["arr"].(float64) != 240000 || an["customers"].(float64) != 2 {
		t.Fatalf("mrr/arr/customers = %v/%v/%v, want 20000/240000/2", an["mrr"], an["arr"], an["customers"])
	}
	if an["one_time_12m"].(float64) != 250000 {
		t.Fatalf("one-time revenue = %v", an["one_time_12m"])
	}
	if n := len(an["series"].([]any)); n != 6 {
		t.Fatalf("series has %d months, want 6", n)
	}
}

func TestACancelledSaleLeavesMRRAndCanComeBack(t *testing.T) {
	h, _, _ := teamServer(t)
	_, sale := call(t, h, "POST", "/v1/sales", `{"customer":"Rahul Verma","billing":"monthly","amount":5000}`, token)
	id := sale["id"].(string)

	if code, s := call(t, h, "PATCH", "/v1/sales/"+id, `{"status":"cancelled","cancel_reason":"Price"}`, token); code != 200 || s["ended_on"] == nil {
		t.Fatalf("cancel: %d %v", code, s)
	}
	if _, an := call(t, h, "GET", "/v1/analytics", "", token); an["mrr"].(float64) != 0 {
		t.Fatalf("mrr after cancel = %v, want 0", an["mrr"])
	}
	if code, s := call(t, h, "PATCH", "/v1/sales/"+id, `{"status":"active"}`, token); code != 200 || s["ended_on"] != nil || s["cancel_reason"] != "" {
		t.Fatalf("reactivate: %d %v", code, s)
	}
	if _, an := call(t, h, "GET", "/v1/analytics", "", token); an["mrr"].(float64) != 5000 {
		t.Fatalf("mrr after reactivate = %v, want 5000", an["mrr"])
	}
}

func TestSalesAreCheckedAndOnlyAdminsDeleteThem(t *testing.T) {
	h, _, _ := teamServer(t)
	for _, body := range []string{
		`{"customer":"","billing":"monthly","amount":100}`,
		`{"customer":"A","billing":"weekly","amount":100}`,
		`{"customer":"A","billing":"monthly","amount":0}`,
		`{"customer":"A","billing":"monthly","amount":100,"email":"nope"}`,
		`{"customer":"A","billing":"monthly","amount":100,"started_on":"yesterday"}`,
	} {
		if code, _ := call(t, h, "POST", "/v1/sales", body, token); code != 422 {
			t.Fatalf("%s: got %d, want 422", body, code)
		}
	}
	_, sale := call(t, h, "POST", "/v1/sales", `{"customer":"A","billing":"one_time","amount":100}`, token)
	id := sale["id"].(string)

	rep := invite(t, h, "Sam Sales", "sam@example.com", "sales")
	if code, _ := call(t, h, "DELETE", "/v1/sales/"+id, "", rep); code != 403 {
		t.Fatalf("a salesperson deleting: %d, want 403", code)
	}
	if code, _ := call(t, h, "GET", "/v1/analytics", "", rep); code != 200 {
		t.Fatalf("a salesperson reading analytics: %d, want 200", code)
	}
	if w := do(h, "DELETE", "/v1/sales/"+id, "", bearer()...); w.Code != 204 {
		t.Fatalf("admin delete: %d", w.Code)
	}
	if code, _ := call(t, h, "GET", "/v1/sales/"+id, "", token); code != 404 {
		t.Fatalf("deleted sale: %d, want 404", code)
	}
}
