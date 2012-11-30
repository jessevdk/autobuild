TARGET = autobuild
SOURCES = $(wildcard *.go)

V =

ifneq ($(V),)
vecho =
veecho =
else
vecho = @echo [$1] $2;
veecho = echo [$1] $2;
endif

GC = go

RESOURCES = $(wildcard resources/*)

SECTIONS = $(foreach i,$(RESOURCES),--add-section autobuild_res_$(notdir $(i))=$(i))

$(TARGET): $(SOURCES) $(RESOURCES)
	$(call vecho,GC,$@) $(GC) build -o $@ $(SOURCES) && \
	objcopy $(SECTIONS) $(TARGET)

clean:
	$(call vecho,CLEAN,$(TARGET)) rm -f $(TARGET)
