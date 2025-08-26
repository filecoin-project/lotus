package types

import (
	"math/big"

	"golang.org/x/crypto/blake2b"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

type ElectionProof struct {
	WinCount int64
	VRFProof []byte
}

const precision = 256

var (
	expNumCoef  []*big.Int
	expDenoCoef []*big.Int
)

func init() {
	parse := func(coefs []string) []*big.Int {
		out := make([]*big.Int, len(coefs))
		for i, coef := range coefs {
			c, ok := new(big.Int).SetString(coef, 10)
			if !ok {
				panic("could not parse exp parameter")
			}
			// << 256 (Q.0 to Q.256), >> 128 to transform integer params to coefficients
			c = c.Lsh(c, precision-128)
			out[i] = c
		}
		return out
	}

	// parameters are in integer format,
	// coefficients are *2^-128 of that
	num := []string{
		"-648770010757830093818553637600",
		"67469480939593786226847644286976",
		"-3197587544499098424029388939001856",
		"89244641121992890118377641805348864",
		"-1579656163641440567800982336819953664",
		"17685496037279256458459817590917169152",
		"-115682590513835356866803355398940131328",
		"340282366920938463463374607431768211456",
	}
	expNumCoef = parse(num)

	deno := []string{
		"1225524182432722209606361",
		"114095592300906098243859450",
		"5665570424063336070530214243",
		"194450132448609991765137938448",
		"5068267641632683791026134915072",
		"104716890604972796896895427629056",
		"1748338658439454459487681798864896",
		"23704654329841312470660182937960448",
		"259380097567996910282699886670381056",
		"2250336698853390384720606936038375424",
		"14978272436876548034486263159246028800",
		"72144088983913131323343765784380833792",
		"224599776407103106596571252037123047424",
		"340282366920938463463374607431768211456",
	}
	expDenoCoef = parse(deno)
}

// expneg accepts x in Q.256 format and computes e^-x.
// It is most precise within [0, 1.725) range, where error is less than 3.4e-30.
// Over the [0, 5) range its error is less than 4.6e-15.
// Output is in Q.256 format.
func expneg(x *big.Int) *big.Int {
	// exp is approximated by rational function
	// polynomials of the rational function are evaluated using Horner's method
	num := polyval(expNumCoef, x)   // Q.256
	deno := polyval(expDenoCoef, x) // Q.256

	num = num.Lsh(num, precision) // Q.512
	return num.Div(num, deno)     // Q.512 / Q.256 => Q.256
}

// polyval evaluates a polynomial given by coefficients `p` in Q.256 format
// at point `x` in Q.256 format. Output is in Q.256.
// Coefficients should be ordered from the highest order coefficient to the lowest.
func polyval(p []*big.Int, x *big.Int) *big.Int {
	// evaluation using Horner's method
	res := new(big.Int).Set(p[0]) // Q.256
	tmp := new(big.Int)           // big.Int.Mul doesn't like when input is reused as output
	for _, c := range p[1:] {
		tmp = tmp.Mul(res, x)         // Q.256 * Q.256 => Q.512
		res = res.Rsh(tmp, precision) // Q.512 >> 256 => Q.256
		res = res.Add(res, c)
	}

	return res
}

// computes lambda in Q.256
func lambda(power, totalPower *big.Int) *big.Int {
	blocksPerEpoch := NewInt(buildconstants.BlocksPerEpoch)
	lam := new(big.Int).Mul(power, blocksPerEpoch.Int)   // Q.0
	lam = lam.Lsh(lam, precision)                        // Q.256
	lam = lam.Div(lam /* Q.256 */, totalPower /* Q.0 */) // Q.256
	return lam
}

var MaxWinCount = 3 * int64(buildconstants.BlocksPerEpoch)

type poiss struct {
	lam  *big.Int
	pmf  *big.Int
	icdf *big.Int

	tmp *big.Int // temporary variable for optimization

	k uint64
}

// newPoiss starts poisson inverted CDF
// lambda is in Q.256 format
// returns (instance, `1-poisscdf(0, lambda)`)
// CDF value returned is reused when calling `next`
func newPoiss(lambda *big.Int) (*poiss, *big.Int) {

	// pmf(k) = (lambda^k)*(e^lambda) / k!
	// k = 0 here, so it simplifies to just e^-lambda
	elam := expneg(lambda) // Q.256
	pmf := new(big.Int).Set(elam)

	// icdf(k) = 1 - ∑ᵏᵢ₌₀ pmf(i)
	// icdf(0) = 1 - pmf(0)
	icdf := big.NewInt(1)
	icdf = icdf.Lsh(icdf, precision) // Q.256
	icdf = icdf.Sub(icdf, pmf)       // Q.256

	k := uint64(0)

	p := &poiss{
		lam: lambda,
		pmf: pmf,

		tmp:  elam,
		icdf: icdf,

		k: k,
	}

	return p, icdf
}

// next computes `k++, 1-poisscdf(k, lam)`
// return is in Q.256 format
func (p *poiss) next() *big.Int {
	// incrementally compute next pmf and icdf

	// pmf(k) = (lambda^k)*(e^lambda) / k!
	// so pmf(k) = pmf(k-1) * lambda / k
	p.k++
	p.tmp.SetUint64(p.k) // Q.0

	// calculate pmf for k
	p.pmf = p.pmf.Div(p.pmf, p.tmp) // Q.256 / Q.0 => Q.256
	// we are using `tmp` as target for multiplication as using an input as output
	// for Int.Mul causes allocations
	p.tmp = p.tmp.Mul(p.pmf, p.lam)     // Q.256 * Q.256 => Q.512
	p.pmf = p.pmf.Rsh(p.tmp, precision) // Q.512 >> 256 => Q.256

	// calculate output
	// icdf(k) = icdf(k-1) - pmf(k)
	p.icdf = p.icdf.Sub(p.icdf, p.pmf) // Q.256
	return p.icdf
}

// ComputeWinCount uses VRFProof to compute number of wins
// The algorithm is based on Algorand's Sortition with Binomial distribution
// replaced by Poisson distribution.
func (ep *ElectionProof) ComputeWinCount(power BigInt, totalPower BigInt) int64 {
	h := blake2b.Sum256(ep.VRFProof)

	lhs := BigFromBytes(h[:]).Int // 256bits, assume Q.256 so [0, 1)

	// We are calculating upside-down CDF of Poisson distribution with
	// rate λ=power*E/totalPower
	// Steps:
	//  1. calculate λ=power*E/totalPower
	//  2. calculate elam = exp(-λ)
	//  3. Check how many times we win:
	//    j = 0
	//    pmf = elam
	//    rhs = 1 - pmf
	//    for h(vrf) < rhs: j++; pmf = pmf * lam / j; rhs = rhs - pmf

	lam := lambda(power.Int, totalPower.Int) // Q.256

	p, rhs := newPoiss(lam)

	var j int64
	for lhs.Cmp(rhs) < 0 && j < MaxWinCount {
		rhs = p.next()
		j++
	}

	return j
}
