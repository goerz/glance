import Foundation

enum FileError: Error {
	case fileAttributeError(path: String, message: String)
	case fileNotFoundError(path: String)
	case fileReadError(path: String, message: String)
}

extension FileError: LocalizedError {
	var errorDescription: String? {
		switch self {
			case let .fileAttributeError(path, message):
				return NSLocalizedString(
					"Could not get attributes for file at path \(path): \(message)",
					comment: ""
				)
			case let .fileNotFoundError(path):
				return NSLocalizedString("Could not find file at path \(path)", comment: "")
			case let .fileReadError(path, message):
				return NSLocalizedString(
					"Could not read file at path \(path): \(message)",
					comment: ""
				)
		}
	}
}

/// Utility class for reading the content and metadata of the corresponding file.
class File {
	let archiveExtensions = ["7z", "tar", "tar.gz", "tgz", "zip"]
	let fileManager = FileManager.default

	var attributes: [FileAttributeKey: Any]
	var isDirectory: Bool
	var path: String
	var url: URL

	var isArchive: Bool { archiveExtensions.contains(url.pathExtension) }
	var size: Int { attributes[.size] as? Int ?? 0 }

	/// Looks for a file at the provided URL and saves its metadata as object properties.
	init(url: URL) throws {
		self.url = url
		path = url.path

		// Check whether the provided URL points to a directory
		var isDirectoryObjC: ObjCBool = false
		guard fileManager.fileExists(atPath: path, isDirectory: &isDirectoryObjC) else {
			throw FileError.fileNotFoundError(path: path)
		}
		isDirectory = isDirectoryObjC.boolValue

		// Read file attributes (e.g. file size)
		do {
			attributes = try fileManager.attributesOfItem(atPath: path)
		} catch let error as NSError {
			throw FileError.fileAttributeError(path: path, message: error.localizedDescription)
		}
	}

	/// Reads and returns the file's content as an UTF-8 string.
	func read() throws -> String {
		do {
			return try String(contentsOf: url, encoding: .utf8)
		} catch {
			throw FileError.fileReadError(path: path, message: error.localizedDescription)
		}
	}

	/// Reads and returns the content of a sibling file whose name is this file's name plus
	/// `suffix`, or `nil` if there is no such file or it cannot be read.
	///
	/// A Quick Look extension is sandboxed to the file it was asked to preview, so reading a
	/// sibling depends on the path exception in `QLPlugin.entitlements`. Note that the failure is
	/// on `open`, not on `stat`: `fileExists` reports true for a file we are not allowed to read,
	/// so the read has to be attempted rather than predicted.
	func readSibling(suffix: String) -> String? {
		try? String(contentsOf: URL(fileURLWithPath: path + suffix), encoding: .utf8)
	}

	/// Reads the file's first line, without reading the rest of it. Used to tell file formats apart
	/// by their magic first line. Returns `nil` if the file cannot be read or is not valid UTF-8.
	static func readFirstLine(url: URL, maxLength: Int = 1024) -> String? {
		guard let handle = try? FileHandle(forReadingFrom: url) else {
			return nil
		}
		defer { try? handle.close() }

		guard let data = try? handle.read(upToCount: maxLength), !data.isEmpty else {
			return nil
		}

		// Decode only up to the first newline, so that a file whose later bytes are not UTF-8
		// still yields its first line.
		let lineData = data.prefix(while: { $0 != UInt8(ascii: "\n") })
		guard let line = String(data: lineData, encoding: .utf8) else {
			return nil
		}
		return line.trimmingCharacters(in: CharacterSet(charactersIn: "\r"))
	}
}
