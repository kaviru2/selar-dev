# Issue #5 research protocol: source ingestion and multimodal memory

Status: implementation protocol; hypotheses are not product claims.

## Research questions

1. How faithfully do PDF, public-web, and pasted-text adapters preserve semantic blocks, figures, captions, metadata, and source locations?
2. Does multimodal retrieval improve evidence retrieval on visual-answer questions without degrading prose-only retrieval?
3. Do relevant original-source visuals used as retrieval cues improve delayed recall or transfer relative to matched text-only practice?
4. Which representation is most effective: text, visual, aggregated text-image, or late fusion?

## Required comparisons

- Existing lexical + text-dense + graph retrieval baseline.
- Gemini Embedding 2 text-only learning objects.
- Visual/page learning objects.
- Aggregated text-image learning objects.
- Late fusion with each signal retained for audit.

Metrics must be reported overall and by source type and answer modality. Primary retrieval outcomes are Recall@k and nDCG; MRR, cost, latency, ingestion fidelity, and asset false-positive rates are secondary outcomes. The deterministic evaluator is in `services/selar-worker/evaluation.py`.

## Provenance

Each source, snapshot, ingestion run, block, chunk, asset, and experiment item has a stable identifier. Ingestion runs record extractor/model versions, dimensions, errors, timestamps, and metrics. Gold corpora must record license/rights, immutable hashes, labels, adjudication, prompts, code revision, and random seeds.

Vendor documentation may support implementation facts but must not be cited as evidence that a learning intervention works. Confirmatory analyses require preregistered outcomes, exclusions, randomization, power analysis, and statistical models. Null and negative findings are retained.

## Grounded chat-to-graph protocol

Chat may propose a new concept only when a bounded concept phrase can be extracted from the user's question and matched directly in at least one cited source chunk after case and separator normalization. Generated answer text and feedback comments are never treated as factual graph evidence. New concepts enter the graph as `candidate`, retain the originating message, document, chunk, query term, binding method, and reducer version, and require explicit human confirmation or rejection.

Candidate chat relationships are limited to three per turn and connect the newly discovered concept to independently bound concepts from different cited chunks. They remain candidates until repeated or independent evidence, or an explicit human decision, changes their state. Evaluation must report candidate precision, candidate recall, unsupported-node rate, normalization collision rate, acceptance/rejection rate, and provenance completeness. A held-out entity set should include punctuation, casing, acronym, and near-name variants.

## Scholarly basis

- Mayer, R. E., & Gallini, J. K. (1990). When is an illustration worth ten thousand words? *Journal of Educational Psychology, 82*(4), 715–726. https://doi.org/10.1037/0022-0663.82.4.715
- Harp, S. F., & Mayer, R. E. (1998). How seductive details do their damage. *Journal of Educational Psychology, 90*(3), 414–434. https://doi.org/10.1037/0022-0663.90.3.414
- Xie, H., et al. (2017). Cueing in multimedia learning: meta-analysis. *PLOS ONE, 12*(8), e0183884. https://doi.org/10.1371/journal.pone.0183884
- Karpicke, J. D., & Roediger, H. L. (2008). The critical importance of retrieval for learning. *Science, 319*(5865), 966–968. https://doi.org/10.1126/science.1152408
- Cepeda, N. J., et al. (2006). Distributed practice in verbal recall tasks. *Psychological Bulletin, 132*(3), 354–380. https://doi.org/10.1037/0033-2909.132.3.354
- Radford, A., et al. (2021). Learning transferable visual models from natural language supervision. *ICML 2021*. https://proceedings.mlr.press/v139/radford21a.html
- Faysse, M., et al. (2025). ColPali: Efficient document retrieval with vision language models. *ICLR 2025*. https://proceedings.iclr.cc/paper_files/paper/2025/hash/99e9e141aafc314f76b0ca3dd66898b3-Abstract-Conference.html
- Dong, Y., et al. (2025). MMDocIR: Benchmarking multimodal retrieval for long documents. *EMNLP 2025*. https://aclanthology.org/2025.emnlp-main.1576/
