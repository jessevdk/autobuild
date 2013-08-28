SOURCES = genresources.go

include go.make

RESOURCES = $(shell find resources/ -type f)
MANINSTALLDIR = $(DESTDIR)$(mandir)/share/man/man1

genresources.go: $(RESOURCES)
	go run build/makeresources.go --output $@ --strip-prefix resources --compress $^

clean-man:
	rm -f $(TARGET).man .gen-man

clean: clean-man

install: install-man

install-man: $(TARGET).man
	test -z "$(MANINSTALLDIR)" || mkdir -p "$(MANINSTALLDIR)" && \
	install -c -m 644 $(TARGET).man "$(MANINSTALLDIR)/$(TARGET).1"

uninstall: uninstall-man

uninstall-man:
	rm -f "$(INSTALLDIR)/bin/$(TARGET)"; \
	rm -f "$(MANINSTALLDIR)/$(TARGET).1"

.gen-man: $(MANSOURCES)
	(cd man && go build -o ../$@)

$(TARGET).man: .gen-man
	./.gen-man > $@

.PHONY: install-man clean-man uninstall-man
