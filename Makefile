TARGET = autobuild
ALL_SOURCES = $(wildcard *.go)
DESTDIR =
PREFIX = /usr/local

# Remove man.go
SOURCES = $(subst man.go,,$(ALL_SOURCES))
MAN_SOURCES = $(subst autobuild.go,man.go,$(SOURCES))

INSTALLDIR = $(DESTDIR)$(PREFIX)

V =

ifneq ($(V),)
vecho =
veecho =
else
vecho = @echo [$1] $2;
veecho = echo [$1] $2;
endif

GC = go

RESOURCES = $(shell find resources/ -type f)
SECTIONS = $(foreach i,$(RESOURCES),--add-section autobuild_res_$(subst resources/,,$(i))=$(i))

MANINSTALLDIR = $(INSTALLDIR)/share/man/man1

all: $(TARGET) $(TARGET).man

$(TARGET): $(SOURCES) $(RESOURCES)
	$(call vecho,GC,$@) $(GC) build -o $@ $(SOURCES) && \
	$(call veecho,RES,$@) objcopy $(SECTIONS) $(TARGET)

CLEANFILES = $(TARGET) $(TARGET).man .gen-man

clean:
	$(call vecho,CLEAN,$(CLEANFILES)) rm -f $(CLEANFILES)

install: $(TARGET) $(TARGET).man
	test -z "$(INSTALLDIR)/bin" || mkdir -p "$(INSTALLDIR)/bin" && \
	install -c $(TARGET) "$(INSTALLDIR)/bin"; \
	test -z "$(MANINSTALLDIR)" || mkdir -p "$(MANINSTALLDIR)" && \
	install -c -m 644 $(TARGET).man "$(MANINSTALLDIR)/$(TARGET).1"

uninstall:
	rm -f "$(INSTALLDIR)/bin/$(TARGET)"; \
	rm -f "$(MANINSTALLDIR)/$(TARGET).1"

.gen-man: $(MAN_SOURCES)
	$(call vecho,GC,$@) $(GC) build -o $@ $(MAN_SOURCES)

$(TARGET).man: .gen-man
	$(call vecho,MAN,$@) ./.gen-man > $@

.PHONY: install clean all
