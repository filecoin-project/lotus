/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */

package blst

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func decodeP1(m map[string]interface{}) *P1Affine {
	x, err := hex.DecodeString(m["x"].(string)[2:])
	if err != nil {
		fmt.Println(err)
		return nil
	}
	y, err := hex.DecodeString(m["y"].(string)[2:])
	if err != nil {
		fmt.Println(err)
		return nil
	}
	var p1 P1Affine
	p1.x.FromBEndian(x)
	p1.y.FromBEndian(y)
	return &p1
}

func TestG1HashToCurve(t *testing.T) {
	vfile, err := os.Open("hash_to_curve/BLS12381G1_XMD_SHA-256_SSWU_RO_.json")
	if err != nil {
		t.Errorf(err.Error())
	}
	defer vfile.Close()
	buf, err := ioutil.ReadAll(vfile)
	if err != nil {
		t.Errorf(err.Error())
	}

	var vectors map[string]interface{}
	err = json.Unmarshal(buf, &vectors)
	if err != nil {
		t.Errorf(err.Error())
	}

	dst := []byte(vectors["dst"].(string))

	vectorsArr, ok := vectors["vectors"].([]interface{})
	if !ok {
		t.Errorf("Could not cast vectors to an array")
	}

	for _, v := range vectorsArr {
		testMap, ok := v.(map[string]interface{})
		if !ok {
			t.Errorf("Could not cast vector to map")
		}

		msg := []byte(testMap["msg"].(string))
		p1Expected := decodeP1(testMap["P"].(map[string]interface{}))
		p1Hashed := HashToG1(msg, dst).ToAffine()

		if !p1Hashed.Equals(p1Expected) {
			t.Errorf("hashed != expected")
		}
	}
}

func decodeP2(m map[string]interface{}) *P2Affine {
	xArr := strings.Split(m["x"].(string), ",")
	x0, err := hex.DecodeString(xArr[0][2:])
	if err != nil {
		fmt.Println(err)
		return nil
	}
	x1, err := hex.DecodeString(xArr[1][2:])
	if err != nil {
		fmt.Println(err)
		return nil
	}
	yArr := strings.Split(m["y"].(string), ",")
	y0, err := hex.DecodeString(yArr[0][2:])
	if err != nil {
		fmt.Println(err)
		return nil
	}
	y1, err := hex.DecodeString(yArr[1][2:])
	if err != nil {
		fmt.Println(err)
		return nil
	}
	var p2 P2Affine
	p2.x.fp[0].FromBEndian(x0)
	p2.x.fp[1].FromBEndian(x1)
	p2.y.fp[0].FromBEndian(y0)
	p2.y.fp[1].FromBEndian(y1)
	return &p2
}

func TestG2HashToCurve(t *testing.T) {
	vfile, err := os.Open("hash_to_curve/BLS12381G2_XMD_SHA-256_SSWU_RO_.json")
	if err != nil {
		t.Errorf(err.Error())
	}
	defer vfile.Close()
	buf, err := ioutil.ReadAll(vfile)
	if err != nil {
		t.Errorf(err.Error())
	}

	var vectors map[string]interface{}
	err = json.Unmarshal(buf, &vectors)
	if err != nil {
		t.Errorf(err.Error())
	}

	dst := []byte(vectors["dst"].(string))

	vectorsArr, ok := vectors["vectors"].([]interface{})
	if !ok {
		t.Errorf("Could not cast vectors to an array")
	}

	for _, v := range vectorsArr {
		testMap, ok := v.(map[string]interface{})
		if !ok {
			t.Errorf("Could not cast vector to map")
		}

		msg := []byte(testMap["msg"].(string))
		p2Expected := decodeP2(testMap["P"].(map[string]interface{}))
		p2Hashed := HashToG2(msg, dst).ToAffine()

		if !p2Hashed.Equals(p2Expected) {
			t.Errorf("hashed != expected")
		}
	}
}
