package common

import (
	"fmt"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func IsValidSpotAllocationStrategy(strategy string) error {
	for _, v := range ec2types.SpotAllocationStrategy("").Values() {
		if string(v) == strategy {
			return nil
		}
	}
	return fmt.Errorf("invalid spot allocation strategy %q, valid values are either lowest-price, diversified,"+
		"capacity-optimized, capacity-optimized-prioritized, price-capacity-optimized", strategy)
}
