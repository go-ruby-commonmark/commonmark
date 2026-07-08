# frozen_string_literal: true
#
# Pure-Ruby usage of the Commonmark module, as provided by go-embedded-ruby
# (rbgo). Run it with:  rbgo examples/commonmark_usage.rb

require "commonmark"

markdown = <<~MD
  # Dimail

  A **pure-Go** mail API client, with a `Ruby` face.

  - domains
  - mailboxes
  - aliases
MD

# Render Markdown to HTML.
puts Commonmark.to_html(markdown)

# The same is available as a String method.
puts "Inline _emphasis_ and a [link](https://example.gouv.fr).".to_html
