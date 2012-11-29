package main

/*
#include <sys/types.h>
#include <grp.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>

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

static int
lookup_group (char const *group)
{
	struct group *g;

	g = getgrnam (group);

	if (!g)
	{
		return -1;
	}

	return (int)g->gr_gid;
}
*/
import "C"

import (
	"unsafe"
	"fmt"
)

func userIsMemberOfGroup(user string, group string) bool {
	g := C.CString(group)
	u := C.CString(user)

	val := C.find_group_member(g, u)

	C.free(unsafe.Pointer(g))
	C.free(unsafe.Pointer(u))

	return val == 1
}

func lookupGroupId(group string) (uint32, error) {
	g := C.CString(group)
	val := C.lookup_group(g)
	C.free(unsafe.Pointer(g))

	if val == -1 {
		return 0, fmt.Errorf("Group `%s' does not exist", group)
	}

	return uint32(val), nil
}
