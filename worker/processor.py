import os
from docling.document_converter import DocumentConverter

class CodiceParser:
    def __init__(self, allowed_dirs=None):
        # Initialize the heavy converter once in memory
        print("🧠 Initializing Docling engine...")
        self.converter = DocumentConverter()
        
        # Define allowed directories for file processing
        if allowed_dirs is None:
            base_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
            self.allowed_dirs = [
                os.path.abspath(os.path.join(base_dir, "backend", "uploads")),
                os.path.abspath(os.path.join(base_dir, "uploads"))
            ]
        else:
            self.allowed_dirs = [os.path.abspath(d) for d in allowed_dirs]

    def is_safe_path(self, file_path):
        """Check if the file path is strictly within allowed directories."""
        abs_target = os.path.abspath(file_path)
        for allowed in self.allowed_dirs:
            if abs_target.startswith(allowed + os.sep) or abs_target == allowed:
                return True
        return False

    def extract_to_markdown(self, file_path):
        """Converts PDF/EPUB to structured Markdown."""
        if not os.path.exists(file_path):
            raise FileNotFoundError(f"File not found: {file_path}")

        if not self.is_safe_path(file_path):
            raise ValueError(f"Access denied to file outside allowed upload directory: {file_path}")

        print(f"⚙️ Reading and extracting text blocks...")
        
        # Docling performs OCR and layout analysis here
        result = self.converter.convert(file_path)
        
        # Export result to Markdown format
        md_content = result.document.export_to_markdown()
        
        return md_content