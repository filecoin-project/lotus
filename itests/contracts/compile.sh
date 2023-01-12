
find -type f -name \*.sol -print0 |
	xargs -0 -I{} bash -c 'solc --bin   {} |tail -n1 | tr -d "\n" > $(echo {} | sed -e s/.sol$/.hex/)'
