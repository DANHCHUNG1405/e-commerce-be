package seller

import "testing"

func TestShopInput(t *testing.T) {
	in := ShopInput{Name: "Shop", PickupAddress: PickupAddress{RecipientName: "Seller", Phone: "0900000000", AddressLine: "Address", Ward: "Ward", District: "District", Province: "City"}}
	if err := in.Validate(); err != nil {
		t.Fatal(err)
	}
	in.PickupAddress.AddressLine = " "
	if in.Validate() == nil {
		t.Fatal("empty pickup address")
	}
}
