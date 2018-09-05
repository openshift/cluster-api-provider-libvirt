// Copyright © 2018 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cidr

import (
	"fmt"
	"net"
)

// GenerateIP takes a parent CIDR range and turns it into a host IP address with
// the given host number.
//
// For example, 10.3.0.0/16 with a host number of 2 gives 10.3.0.2.
func GenerateIP(base *net.IPNet, num int) (net.IP, error) {
	ip := base.IP
	mask := base.Mask

	parentLen, addrLen := mask.Size()
	hostLen := addrLen - parentLen

	maxHostNum := uint64(1<<uint64(hostLen)) - 1

	numUint64 := uint64(num)
	if num < 0 {
		numUint64 = uint64(-num) - 1
		num = int(maxHostNum - numUint64)
	}

	if numUint64 > maxHostNum {
		return nil, fmt.Errorf("prefix of %d does not accommodate a host numbered %d", parentLen, num)
	}
	var bitlength int
	if ip.To4() != nil {
		bitlength = 32
	} else {
		bitlength = 128
	}
	return insertNumIntoIP(ip, num, bitlength), nil
}
