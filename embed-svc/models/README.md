# Model Download

Download the all-MiniLM-L6-v2 ONNX model and tokenizer from HuggingFace:

```bash
mkdir -p models/all-MiniLM-L6-v2
wget https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx \
  -O models/all-MiniLM-L6-v2/model.onnx
wget https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/tokenizer.json \
  -O models/all-MiniLM-L6-v2/tokenizer.json
```

Or use `make download-model` from the embed-svc directory.
