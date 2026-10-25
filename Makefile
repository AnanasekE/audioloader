TAILWIND := bin/tailwindcss
CSS_IN   := styles/input.css
CSS_OUT  := assets/css/app.css

.PHONY: build generate css css-watch dev

# live reload: rebuilds on .go/.templ/.css changes, browser at :8090
dev:
	air

build: generate css
	go build -o audioloader .

generate:
	templ generate

css:
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT) --minify

css-watch:
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT) --watch
