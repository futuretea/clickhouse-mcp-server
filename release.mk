NPM_PACKAGE ?= @futuretea/clickhouse-mcp-server
NPM_PACKAGE_DIRECTORY ?= clickhouse-mcp-server
NPM_ROOT ?= npm
NPM_VERSION ?= $(shell git describe --tags --always 2>/dev/null | sed 's/^v//')
NPM_PUBLISH_FLAGS ?= --access=public --tag=latest
NPM_PACK_DIR ?= dist/npm-packages
NPM ?= npm
NODE ?= node
NPM_INVENTORY ?= .github/ci/release-platforms.json
NPM_SCOPE ?= $(dir $(NPM_PACKAGE))
NPM_NATIVE_PLATFORMS := $(shell jq -er '.native.include[] | "\(.goos)-\(.goarch)"' "$(NPM_INVENTORY)")
NPM_NATIVE_PACKAGE_DIRECTORIES := $(addprefix $(NPM_PACKAGE_DIRECTORY)-,$(NPM_NATIVE_PLATFORMS))
NPM_PACKAGE_DIRECTORIES := $(NPM_NATIVE_PACKAGE_DIRECTORIES) $(NPM_PACKAGE_DIRECTORY)

.PHONY: npm-verify-binaries npm-prepare-packages npm-create-tarballs npm-verify-tarballs npm-publish

npm-verify-binaries:
	@set -eu; \
		for platform in $(NPM_NATIVE_PLATFORMS); do \
			package_dir="$(NPM_ROOT)/$(NPM_PACKAGE_DIRECTORY)-$$platform"; \
			goos="$${platform%-*}"; \
			goarch="$${platform##*-}"; \
			nodeos="$$goos"; \
			nodearch="$$goarch"; \
			if [ "$$nodeos" = windows ]; then nodeos=win32; fi; \
			if [ "$$nodearch" = amd64 ]; then nodearch=x64; fi; \
			jq -e --arg name "$(NPM_SCOPE)$(NPM_PACKAGE_DIRECTORY)-$$platform" --arg os "$$nodeos" --arg cpu "$$nodearch" \
				'.name == $$name and .os == [$$os] and .cpu == [$$cpu]' "$$package_dir/package.json" >/dev/null; \
			case "$$platform" in \
				windows-*) test -f "$$package_dir/bin/$(BINARY_NAME).exe" ;; \
				*) test -x "$$package_dir/bin/$(BINARY_NAME)" ;; \
			esac; \
		done

npm-prepare-packages: npm-verify-binaries
	test -f README.md
	test -f LICENSE
	cp README.md LICENSE "$(NPM_ROOT)/$(NPM_PACKAGE_DIRECTORY)/"
	@set -eu; for package_dir in $(NPM_NATIVE_PACKAGE_DIRECTORIES); do \
		package_path="$(NPM_ROOT)/$$package_dir"; \
		jq --arg version "$(NPM_VERSION)" '.version = $$version' "$$package_path/package.json" > "$$package_path/package.json.tmp"; \
		mv "$$package_path/package.json.tmp" "$$package_path/package.json"; \
	done
	jq --arg version "$(NPM_VERSION)" --arg scope "$(NPM_SCOPE)" --arg package "$(NPM_PACKAGE_DIRECTORY)" --slurpfile platforms "$(NPM_INVENTORY)" \
		'.version = $$version | .optionalDependencies = (reduce $$platforms[0].native.include[] as $$platform ({}; .[$$scope + $$package + "-" + $$platform.goos + "-" + $$platform.goarch] = $$version))' \
		"$(NPM_ROOT)/$(NPM_PACKAGE_DIRECTORY)/package.json" > "$(NPM_ROOT)/$(NPM_PACKAGE_DIRECTORY)/package.json.tmp"
	mv "$(NPM_ROOT)/$(NPM_PACKAGE_DIRECTORY)/package.json.tmp" "$(NPM_ROOT)/$(NPM_PACKAGE_DIRECTORY)/package.json"

npm-create-tarballs: npm-prepare-packages
	rm -rf "$(NPM_PACK_DIR)"
	mkdir -p "$(NPM_PACK_DIR)"
	@set -eu; for package_dir in $(NPM_PACKAGE_DIRECTORIES); do \
		package_path="$(NPM_ROOT)/$$package_dir"; \
		filename="$$(cd "$$package_path" && $(NPM) pack --json | jq -er '.[0].filename')"; \
		mv "$$package_path/$$filename" "$(NPM_PACK_DIR)/$$package_dir.tgz"; \
	done

npm-verify-tarballs: npm-create-tarballs
	@set -eu; \
		expected_dependencies="$$(jq -cS --arg version "$(NPM_VERSION)" --arg scope "$(NPM_SCOPE)" --arg package "$(NPM_PACKAGE_DIRECTORY)" 'reduce .native.include[] as $$platform ({}; .[$$scope + $$package + "-" + $$platform.goos + "-" + $$platform.goarch] = $$version)' "$(NPM_INVENTORY)")"; \
		actual_dependencies="$$(jq -cS '.optionalDependencies' "$(NPM_ROOT)/$(NPM_PACKAGE_DIRECTORY)/package.json")"; \
		test "$$actual_dependencies" = "$$expected_dependencies"; \
		tar -tzf "$(NPM_PACK_DIR)/$(NPM_PACKAGE_DIRECTORY).tgz" | grep -Fx 'package/bin/index.js' >/dev/null; \
		for platform in $(NPM_NATIVE_PLATFORMS); do \
			package_dir="$(NPM_PACKAGE_DIRECTORY)-$$platform"; \
			suffix=''; \
			if [ "$${platform%-*}" = windows ]; then suffix=.exe; fi; \
			tar -tzf "$(NPM_PACK_DIR)/$$package_dir.tgz" | grep -Fx "package/bin/$(BINARY_NAME)$$suffix" >/dev/null; \
		done

npm-publish: npm-verify-tarballs
	@set -eu; \
		temp_dir="$$(mktemp -d)"; \
		trap 'rm -rf "$$temp_dir"' EXIT; \
		for package_dir in $(NPM_PACKAGE_DIRECTORIES); do \
			package_path="$(NPM_ROOT)/$$package_dir"; \
			package_name="$$(jq -er '.name' "$$package_path/package.json")"; \
			package_version="$$(jq -er '.version' "$$package_path/package.json")"; \
			package_spec="$$package_name@$$package_version"; \
			archive="$(NPM_PACK_DIR)/$$package_dir.tgz"; \
			local_integrity="$$( $(NODE) -e 'const crypto = require("crypto"); const fs = require("fs"); process.stdout.write("sha512-" + crypto.createHash("sha512").update(fs.readFileSync(process.argv[1])).digest("base64"));' "$$archive")"; \
			if $(NPM) view "$$package_spec" dist.integrity --json > "$$temp_dir/view.json" 2> "$$temp_dir/view.err"; then \
				remote_integrity="$$(jq -er 'if type == "string" and length > 0 then . else error("missing dist.integrity") end' "$$temp_dir/view.json")"; \
				if [ "$$local_integrity" != "$$remote_integrity" ]; then printf '%s\n' "Existing $$package_spec does not match the local release artifact" >&2; exit 1; fi; \
				echo "$$package_spec is already published with matching integrity"; \
			elif grep -Eq 'E404|404 Not Found' "$$temp_dir/view.err"; then \
				NPM_PACKAGE_NAME="$$package_name" NPM_PACKAGE_VERSION="$$package_version" $(NPM) publish "$$archive" $(NPM_PUBLISH_FLAGS); \
			else \
				cat "$$temp_dir/view.err" >&2; exit 1; \
			fi; \
		done
