package main

/*
#include <sys/types.h>
#include <grp.h>
#include <stdlib.h>
#include <string.h>

static int
find_group_member (char const *group, char const *member)
{
	struct group *g;
	int i = 0;

	g = getgrnam (group);

	if (!g)
	{
		return 0;
	}

	while (g->gr_mem[i])
	{
		if (strcmp (g->gr_mem[i], member) == 0)
		{
			return 1;
		}

		++i;
	}

	return 0;
}
*/
import "C"

import (
	"unsafe"
)

func userIsMemberOfGroup(user string, group string) bool {
	g := C.CString(group)
	u := C.CString(user)

	val := C.find_group_member(g, u)

	C.free(unsafe.Pointer(g))
	C.free(unsafe.Pointer(u))

	return val == 1
}
