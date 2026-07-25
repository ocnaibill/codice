# Códice 📚

A complete, modern, and open-source system for managing and consuming digital libraries (Homelab). Códice supports fiction books (EPUB), manga/comic collections (CBZ), and features a specialized processing pipeline with Full-Text Search for study PDFs and academic articles.

## 🏗️ Architecture

Códice is built with a focus on performance, resilience, and low resource consumption, ideal for running on local servers (Homelabs).

*   **Backend (API & OPDS):** Go (Golang)
*   **Extraction Worker:** Python + Docling (Asynchronous PDF processing)
*   **Message Queue:** Redis Streams
*   **Database:** PostgreSQL (with GIN indexes for Full-Text searches)
*   **Frontend:** React (Vite) + Tailwind CSS + Zustand

## 🚀 Project Status

Currently in the **structural foundation** phase. Check the [Issues](https://github.com/ocnaibill/codice/issues) to see active development tasks.

## 📄 License

This project is licensed under **GNU AGPLv3** - see the [LICENSE](LICENSE) file for details. Códice is copyleft: feel free to host, use, and modify, as long as any modified source code remains public.