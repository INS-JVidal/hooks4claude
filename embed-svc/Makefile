.PHONY: build run download-model download-onnxruntime

ORT_VERSION := 1.23.2
ORT_LIB_DIR := lib
ORT_LIB := $(ORT_LIB_DIR)/libonnxruntime.so

build: $(ORT_LIB)
	cargo build --release

run: $(ORT_LIB)
	ORT_DYLIB_PATH=$(CURDIR)/$(ORT_LIB) cargo run --release

$(ORT_LIB): download-onnxruntime

download-onnxruntime:
	@if [ ! -f "$(ORT_LIB)" ]; then \
		echo "Downloading ONNX Runtime v$(ORT_VERSION)..."; \
		mkdir -p $(ORT_LIB_DIR); \
		ARCH=$$(uname -m); \
		if [ "$$ARCH" = "x86_64" ]; then \
			ORT_ARCH="x64"; \
		elif [ "$$ARCH" = "aarch64" ]; then \
			ORT_ARCH="aarch64"; \
		else \
			echo "Unsupported architecture: $$ARCH"; exit 1; \
		fi; \
		URL="https://github.com/microsoft/onnxruntime/releases/download/v$(ORT_VERSION)/onnxruntime-linux-$$ORT_ARCH-$(ORT_VERSION).tgz"; \
		echo "Fetching $$URL"; \
		curl -sL "$$URL" | tar xz --strip-components=1 -C $(ORT_LIB_DIR) --wildcards '*/lib/libonnxruntime.so*'; \
		mv $(ORT_LIB_DIR)/lib/libonnxruntime.so* $(ORT_LIB_DIR)/ 2>/dev/null || true; \
		rmdir $(ORT_LIB_DIR)/lib 2>/dev/null || true; \
		ls -la $(ORT_LIB_DIR)/libonnxruntime.so*; \
	fi

download-model:
	mkdir -p models/all-MiniLM-L6-v2
	wget https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx \
		-O models/all-MiniLM-L6-v2/model.onnx
	wget https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/tokenizer.json \
		-O models/all-MiniLM-L6-v2/tokenizer.json
