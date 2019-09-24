syms height;


IV = 153;
HalvingPeriod = (6 * 365 * 24 * 60 * 2);
AdjusmentPeriod = vpa(20160);
Total = vpa(1400e6);

syms coffer(height)
syms R(height);
syms h
%coffer(height) = piecewise(height <= 0, Total, height > 0, Total - symsum(R(h), h, 0, height));
coffer(height) = piecewise(height < 0, Total, height >= 0, coffer(height-1) - R(height));
R(height) = coffer(height-1)/Total * IV;
syms x
r = R(x);

fplot(r, [0, 5]);
fibonacci