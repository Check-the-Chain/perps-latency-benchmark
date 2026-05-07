package booktop

import (
	"fmt"
	"strings"
	"testing"
)

func TestVenueParsersParseTopOfBook(t *testing.T) {
	tests := []struct {
		name  string
		venue string
		data  string
		bid   float64
		ask   float64
		bids  int
		asks  int
	}{
		{
			name:  "hyperliquid",
			venue: "hyperliquid",
			data:  `{"channel":"l2Book","data":{"time":1777966248747,"levels":[[{"px":"100","sz":"5"},{"px":"99","sz":"2"}],[{"px":"101","sz":"3"},{"px":"102","sz":"4"}]]}}`,
			bid:   100,
			ask:   101,
			bids:  2,
			asks:  2,
		},
		{
			name:  "hyperliquid-http-l2book",
			venue: "hyperliquid",
			data:  `{"coin":"BTC","time":1777966248747,"levels":[[{"px":"100","sz":"5"},{"px":"99","sz":"2"}],[{"px":"101","sz":"3"},{"px":"102","sz":"4"}]]}`,
			bid:   100,
			ask:   101,
			bids:  2,
			asks:  2,
		},
		{
			name:  "aster",
			venue: "aster",
			data:  `{"E":1777966248747,"b":[["100","5"],["99","2"]],"a":[["101","3"],["102","4"]]}`,
			bid:   100,
			ask:   101,
			bids:  2,
			asks:  2,
		},
		{
			name:  "lighter",
			venue: "lighter",
			data:  `{"bids":[{"price":"100","size":"5"},{"price":"99","size":"2"}],"asks":[{"price":"101","size":"3"},{"price":"102","size":"4"}]}`,
			bid:   100,
			ask:   101,
			bids:  2,
			asks:  2,
		},
		{
			name:  "pacifica",
			venue: "pacifica",
			data:  `{"channel":"bbo","data":{"s":"BTC","t":1777966248747,"b":"100","B":"5","a":"101","A":"3"}}`,
			bid:   100,
			ask:   101,
			bids:  1,
			asks:  1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := map[string]Parser{
				"hyperliquid": NewHyperliquidParser(),
				"aster":       NewAsterParser(),
				"lighter":     NewLighterParser(),
				"pacifica":    NewPacificaParser(),
			}[test.venue]
			snapshot, ok := parser.Parse([]byte(test.data))
			if !ok {
				t.Fatal("expected snapshot")
			}
			if snapshot.Bid != test.bid || snapshot.Ask != test.ask {
				t.Fatalf("snapshot = %+v", snapshot)
			}
			if len(snapshot.Bids) != test.bids || len(snapshot.Asks) != test.asks {
				t.Fatalf("snapshot depth = %+v", snapshot)
			}
		})
	}
}

func TestExtendedOrderBookAppliesDeltas(t *testing.T) {
	parser := NewExtendedParser()
	snapshot, ok := parser.Parse([]byte(`{
		"type":"SNAPSHOT",
		"data":{
			"b":[{"q":"5.0","p":"100"},{"q":"1.0","p":"99"}],
			"a":[{"q":"3.0","p":"101"},{"q":"2.0","p":"102"}]
		},
		"ts":1777966248747
	}`))
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snapshot.Bid != 100 || snapshot.BidSize != 5 || snapshot.Ask != 101 || snapshot.AskSize != 3 {
		t.Fatalf("snapshot = %+v", snapshot)
	}

	snapshot, ok = parser.Parse([]byte(`{
		"type":"DELTA",
		"data":{
			"b":[{"q":"-5.0","p":"100","c":"0"},{"q":"2.0","p":"98","c":"2.0"}],
			"a":[{"q":"-3.0","p":"101","c":"0"},{"q":"1.0","p":"100.5","c":"1.0"}]
		},
		"ts":1777966248847
	}`))
	if !ok {
		t.Fatal("expected delta snapshot")
	}
	if snapshot.Bid != 99 || snapshot.BidSize != 1 || snapshot.Ask != 100.5 || snapshot.AskSize != 1 {
		t.Fatalf("snapshot after delta = %+v", snapshot)
	}
	if len(snapshot.Bids) != 2 || len(snapshot.Asks) != 2 {
		t.Fatalf("snapshot depth after delta = %+v", snapshot)
	}
}

func TestLighterOrderBookAppliesDeltas(t *testing.T) {
	parser := NewLighterParser()
	snapshot, ok := parser.Parse([]byte(`{
		"channel":"order_book:1",
		"type":"subscribed/order_book",
		"order_book":{
			"bids":[{"price":"100","size":"5"},{"price":"99","size":"1"}],
			"asks":[{"price":"101","size":"3"},{"price":"102","size":"2"}],
			"last_updated_at":1777966248747000
		}
	}`))
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snapshot.Bid != 100 || snapshot.BidSize != 5 || snapshot.Ask != 101 || snapshot.AskSize != 3 {
		t.Fatalf("snapshot = %+v", snapshot)
	}

	snapshot, ok = parser.Parse([]byte(`{
		"channel":"order_book:1",
		"type":"update/order_book",
		"order_book":{
			"bids":[{"price":"100","size":"0.00000"},{"price":"98","size":"4"}],
			"asks":[{"price":"101","size":"0.00000"},{"price":"100.5","size":"1"}],
			"last_updated_at":1777966248847000
		}
	}`))
	if !ok {
		t.Fatal("expected delta snapshot")
	}
	if snapshot.Bid != 99 || snapshot.BidSize != 1 || snapshot.Ask != 100.5 || snapshot.AskSize != 1 {
		t.Fatalf("snapshot after delta = %+v", snapshot)
	}
	if len(snapshot.Bids) != 2 || len(snapshot.Asks) != 2 {
		t.Fatalf("snapshot depth after delta = %+v", snapshot)
	}
}

func TestSnapshotsAreCappedToTopLevels(t *testing.T) {
	parser := NewLighterParser()
	var bids []string
	var asks []string
	for i := 0; i < 25; i++ {
		bids = append(bids, fmt.Sprintf(`{"price":"%d","size":"1"}`, 100-i))
		asks = append(asks, fmt.Sprintf(`{"price":"%d","size":"1"}`, 101+i))
	}
	snapshot, ok := parser.Parse([]byte(fmt.Sprintf(`{
		"type":"subscribed/order_book",
		"order_book":{"bids":[%s],"asks":[%s]}
	}`, strings.Join(bids, ","), strings.Join(asks, ","))))
	if !ok {
		t.Fatal("expected snapshot")
	}
	if len(snapshot.Bids) != maxSnapshotLevels || len(snapshot.Asks) != maxSnapshotLevels {
		t.Fatalf("snapshot depth = bids %d asks %d", len(snapshot.Bids), len(snapshot.Asks))
	}
	if snapshot.Bids[0].Price != 100 || snapshot.Bids[len(snapshot.Bids)-1].Price != 86 {
		t.Fatalf("unexpected capped bids: %+v", snapshot.Bids)
	}
	if snapshot.Asks[0].Price != 101 || snapshot.Asks[len(snapshot.Asks)-1].Price != 115 {
		t.Fatalf("unexpected capped asks: %+v", snapshot.Asks)
	}

	snapshot, ok = parser.Parse([]byte(`{
		"type":"update/order_book",
		"order_book":{
			"bids":[{"price":"50","size":"1"}],
			"asks":[{"price":"200","size":"1"}]
		}
	}`))
	if !ok {
		t.Fatal("expected delta snapshot")
	}
	if len(snapshot.Bids) != maxSnapshotLevels || len(snapshot.Asks) != maxSnapshotLevels {
		t.Fatalf("delta snapshot depth = bids %d asks %d", len(snapshot.Bids), len(snapshot.Asks))
	}
	if snapshot.Bids[len(snapshot.Bids)-1].Price == 50 || snapshot.Asks[len(snapshot.Asks)-1].Price == 200 {
		t.Fatalf("retained levels outside top %d: %+v %+v", maxSnapshotLevels, snapshot.Bids, snapshot.Asks)
	}
}
