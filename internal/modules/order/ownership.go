package order

func RequireOrderOwner(checkoutOrder *CheckoutOrder, customerID string) error {
	if checkoutOrder == nil || customerID == "" || checkoutOrder.CustomerID == nil || *checkoutOrder.CustomerID != customerID {
		return ErrOrderNotFound
	}

	return nil
}
