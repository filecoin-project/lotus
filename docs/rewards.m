
HalvingPeriod = vpa(6 * 365 * 24 * 60 * 2);
AdjusmentPeriod = vpa(20160);
Decay = exp(log(vpa(0.5)) / HalvingPeriod * AdjusmentPeriod);
Total = vpa(1400e6);
IV = Total * (1-Decay) / AdjusmentPeriod;


syms R(x)
R(x) = IV * (Decay.^floor(x/AdjusmentPeriod));

Hmax = HalvingPeriod*5;
heights = linspace(0, Hmax, Hmax/AdjusmentPeriod);
Rewards = R(heights);

R2 = zeros(size(heights));

coffer = Total;
for h = 1:size(heights,2)
  k = IV*(coffer/Total);
  coffer = coffer - k*AdjusmentPeriod;
  R2(h) = k;
end
hYears = heights/2/60/24/365;
plot(hYears, Rewards, 'go', hYears, R2, 'r-')
legend('formula', 'incremental');

for6y = Rewards(1:HalvingPeriod/AdjusmentPeriod)*AdjusmentPeriod;
inc6t = R2(1:HalvingPeriod/AdjusmentPeriod)*AdjusmentPeriod;
