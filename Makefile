.PHONY: build vendor clean sync-skills

# Sync vendor before build to avoid Go version mismatch
vendor:
	go mod tidy
	go mod vendor

build: vendor
	CGO_ENABLED=0 go build -mod=vendor -o bin/hub ./cmd/hub/

sync-skills:
	@echo "No bundled private skills are included in the open-source tree."
	@echo "Synced $$(find data/skills -name 'SKILL.md' | wc -l) skills"

clean:
	rm -rf bin/
