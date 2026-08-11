import Foundation
import os.log

/// Preview for Pluto.jl notebooks: `.jl` files whose first line is `### A Pluto.jl notebook ###`.
///
/// A notebook's cell outputs live in a sibling file, `<notebook>.jl.pluto-cache.toml`, written by
/// SpaceStation. When it can be read, the preview shows the computed results — rendered Markdown,
/// plots, tables and errors — the way Pluto itself does. When it cannot, the notebook still
/// previews as prose and syntax-highlighted code, which is the common case: most notebooks on disk
/// have never been run.
class PlutoPreview: Preview {
	/// Suffix of the output-cache sidecar written by SpaceStation.
	private static let cacheSuffix = ".pluto-cache.toml"

	/// Upper bound on the sidecar. A notebook full of plots can produce a large cache, and every
	/// image in it ends up base64-encoded into the preview document.
	private static let maxCacheSize = 32_000_000

	private let chromaStylesheetURL = Bundle.main.url(
		forResource: "shared-chroma",
		withExtension: "css"
	)
	private let mainStylesheetURL = Bundle.main.url(
		forResource: "pluto-main",
		withExtension: "css"
	)
	private let katexAutoRenderScriptURL = Bundle.main.url(
		forResource: "jupyter-katex-auto-render.min",
		withExtension: "js"
	)
	private let katexScriptURL = Bundle.main.url(
		forResource: "jupyter-katex.min",
		withExtension: "js"
	)
	private let katexStylesheetURL = Bundle.main.url(
		forResource: "jupyter-katex.min",
		withExtension: "css"
	)

	required init() {}

	/// Returns whether the file at the given URL is a Pluto notebook, by checking for the header
	/// Pluto writes as the very first line. Only the first line is read.
	static func isPlutoNotebook(fileURL: URL) -> Bool {
		File.readFirstLine(url: fileURL) == "### A Pluto.jl notebook ###"
	}

	private func getHTML(file: File) throws -> String {
		var source: String
		do {
			source = try file.read()
		} catch {
			os_log(
				"Could not read Pluto notebook file: %{public}s",
				log: Log.parse,
				type: .error,
				error.localizedDescription
			)
			throw error
		}

		let cache = getCache(file: file)

		do {
			return try HTMLRenderer.renderPlutoNotebook(source, cache: cache)
		} catch {
			os_log(
				"Could not generate Pluto notebook HTML: %{public}s",
				log: Log.render,
				type: .error,
				error.localizedDescription
			)
			throw error
		}
	}

	/// Reads the output-cache sidecar, returning an empty string when it is missing, unreadable or
	/// implausibly large. None of those is an error: the notebook simply previews without outputs.
	private func getCache(file: File) -> String {
		guard let cache = file.readSibling(suffix: Self.cacheSuffix) else {
			os_log(
				"No readable output cache for %{public}s, rendering code only",
				log: Log.parse,
				type: .info,
				file.path
			)
			return ""
		}

		if cache.utf8.count > Self.maxCacheSize {
			os_log(
				"Output cache for %{public}s is too large (%{public}d bytes), rendering code only",
				log: Log.parse,
				type: .info,
				file.path,
				cache.utf8.count
			)
			return ""
		}

		return cache
	}

	private func getStylesheets() -> [Stylesheet] {
		var stylesheets = [Stylesheet]()

		// Main Pluto stylesheet (adapted from Pluto's own frontend)
		if let mainStylesheetURL = mainStylesheetURL {
			stylesheets.append(Stylesheet(url: mainStylesheetURL))
		} else {
			os_log("Could not find main Pluto stylesheet", log: Log.render, type: .error)
		}

		// Chroma stylesheet (for cell input syntax highlighting)
		if let chromaStylesheetURL = chromaStylesheetURL {
			stylesheets.append(Stylesheet(url: chromaStylesheetURL))
		} else {
			os_log("Could not find Chroma stylesheet", log: Log.render, type: .error)
		}

		// KaTeX stylesheet (for rendering LaTeX math)
		if let katexStylesheetURL = katexStylesheetURL {
			stylesheets.append(Stylesheet(url: katexStylesheetURL))
		} else {
			os_log("Could not find KaTeX stylesheet", log: Log.render, type: .error)
		}

		return stylesheets
	}

	private func getScripts() -> [Script] {
		var scripts = [Script]()

		// KaTeX library (for rendering LaTeX math)
		if let katexScriptURL = katexScriptURL {
			scripts.append(Script(url: katexScriptURL))
		} else {
			os_log("Could not find KaTeX script", log: Log.render, type: .error)
		}

		// KaTeX auto-renderer (finds LaTeX math on the page and calls KaTeX on it)
		if let katexAutoRenderScriptURL = katexAutoRenderScriptURL {
			scripts.append(Script(url: katexAutoRenderScriptURL))
		} else {
			os_log("Could not find KaTeX auto-render script", log: Log.render, type: .error)
		}

		// Pluto renders math with MathJax and wraps it as `<p class="tex">$$…$$</p>`, using `$$`
		// for inline math too. Markdown cells additionally use single `$`, which KaTeX does not
		// recognise by default. Cell input is excluded: a `$` in Julia code is string
		// interpolation, not math.
		scripts.append(Script(content: """
		renderMathInElement(document.body, {
			delimiters: [
				{ left: "$$", right: "$$", display: true },
				{ left: "\\\\[", right: "\\\\]", display: true },
				{ left: "\\\\(", right: "\\\\)", display: false },
				{ left: "$", right: "$", display: false },
			],
			ignoredTags: ["script", "noscript", "style", "textarea", "pre", "code", "option"],
			ignoredClasses: ["chroma"],
			throwOnError: false,
		});
		"""))

		return scripts
	}

	func createPreviewVC(file: File) throws -> PreviewVC {
		WebPreviewVC(
			html: try getHTML(file: file),
			stylesheets: getStylesheets(),
			scripts: getScripts()
		)
	}
}
