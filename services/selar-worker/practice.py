"""Owner-private practice. Never reads or writes formal quiz instruments.

Exact provenance and a deterministic answer-in-quote check are publication
gates, not proof of truth or mastery. Documents and learner responses are
untrusted data, not policy.
"""
import hashlib
import json
import math
import re

VERSION = 'practice-grounding-v2'
SUPPORT_CHECK = 'deterministic-quote-support-v1'
EXACT_RECALL = 'deterministic-exact-recall-v1'

NEGATIONS = {'not', 'no', 'never', 'none', 'nor', 'neither', 'cannot', 'without', 'nothing', 'nobody'}
STOP_WORDS = {
    'a', 'an', 'the', 'of', 'to', 'in', 'on', 'for', 'and', 'or', 'by', 'with', 'as', 'at', 'from',
    'is', 'are', 'was', 'were', 'be', 'been', 'being', 'it', 'its', 'this', 'that', 'these', 'those',
    'which', 'who', 'whom', 'what', 'their', 'they', 'them', 'there', 'than', 'then', 'so', 'such',
    'do', 'does', 'did', 'has', 'have', 'had', 'can', 'could', 'may', 'might', 'will', 'would',
    'should', 'into', 'about', 'also', 'both', 'each', 'via', 'i', 'we', 'our', 'my', 'me', 'you',
}


class Unavailable(Exception):
    pass


def digest(text):
    return hashlib.sha256(text.encode()).hexdigest()


def _stem(word):
    for suffix in ('ing', 'ed', 'es', 's', 'e'):
        if len(word) > 4 and word.endswith(suffix):
            return word[:-len(suffix)]
    return word


def _tokens(text):
    words = re.findall(r"[a-z0-9]+", re.sub(r"n['\u2019]t\b", ' not', str(text).lower()))
    return [w for w in words if w not in STOP_WORDS]


def key_terms(text):
    """Stemmed content words (negations excluded) of ``text``."""
    return {_stem(w) for w in _tokens(text) if w not in NEGATIONS}


def negated(text):
    return any(w in NEGATIONS for w in _tokens(text))


def answer_supported_by_quote(answer, quote):
    """Deterministic support gate replacing the former second model call.

    Every key term of the reference answer must occur in the exact source quote,
    and the answer and quote must agree on negation. Paraphrased answers are
    rejected (fail closed) rather than trusted.
    """
    terms = key_terms(answer)
    if not terms or not terms <= key_terms(quote):
        return False
    if negated(answer):
        return negated(quote)
    # A positive answer is unsupported when the quote negates its predicate
    # ("X does not reduce error"); an unrelated negation elsewhere is fine.
    words = _tokens(quote)
    return not any(word in NEGATIONS and _stem(following) in terms
                   for word, following in zip(words, words[1:]))


def exact_recall_feedback(payload, response):
    """Full credit without a model call when recall restates the reference answer.

    Returns None (defer to the model for free-recall grading) unless every key
    term of the reference answer is present, negation matches, and the response
    is not padded far beyond the answer (resists keyword stuffing).
    """
    terms = key_terms(payload['answer'])
    said = key_terms(response)
    if (not terms or not terms <= said or negated(response) != negated(payload['answer'])
            or len(said) > max(3 * len(terms), len(terms) + 12)):
        return None
    return {'score': 1, 'confident': True, '_model': EXACT_RECALL,
            'feedback': 'Your recall contains every key term of the reference answer. '
                        'Compare it with the source quote below.'}


async def generate_item(source, model):
    draft = await model('generate', {'passage': source['content']})
    if not isinstance(draft, dict):
        raise Unavailable('Invalid generation; retry practice')
    quote = draft.get('quote')
    if (not isinstance(quote, str) or not quote.strip() or quote not in source['content']
            or not source['locator'] or not all(isinstance(draft.get(k), str) and draft[k].strip()
                                               for k in ('question', 'answer'))):
        raise Unavailable('No exact source witness; retry practice')
    if not answer_supported_by_quote(draft['answer'], quote):
        raise Unavailable('Reference answer is not supported by the exact source quote')
    return {**{k: draft[k][:8000] for k in ('question', 'answer', 'quote')},
            'chunk_id': str(source['id']), 'chunk_hash': digest(source['content']),
            'locator': json.loads(source['locator']) if isinstance(source['locator'], str) else source['locator'],
            'label': 'AI-generated practice', 'version': VERSION,
            'generation_model': draft.get('_model', 'unspecified'), 'verifier_model': SUPPORT_CHECK,
            'grounding': 'exact quote + deterministic answer-in-quote check (not established truth)'}


def validated_feedback(result):
    score = result.get('score')
    valid = (type(score) in (int, float) and math.isfinite(score) and 0 <= score <= 1
             and result.get('confident') is True)
    return {'score': score if valid else None, 'provisional': not valid,
            'feedback': str(result.get('feedback', 'Uncertain AI feedback; unscored'))[:4000],
            'version': VERSION, 'model': result.get('_model', 'unspecified'),
            'label': 'AI practice feedback, not a formal assessment'}
