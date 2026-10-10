"use client";
import {useEffect,useState} from 'react';
import Link from 'next/link';
import {quizApi} from '@/lib/quiz/client';
import {PageHeader} from '@/components/ui/Card';
import {PracticeSkeleton} from '@/components/PracticeSkeleton';
import {useSelarUserId} from '@/lib/context';
import {loadPractice,peekPractice} from '@/lib/practice-cache';
interface ProgressData {warmup:number;reading_check:number;review:number;exposed:number;unscored:number;delayed_unassisted:number;delayed_scored:number;delayed_mean_score:number|null;streak:number;due:number}
export default function Progress(){
 const userId=useSelarUserId();
 // Show this account's last report at once (if any) and refresh it in the background.
 const [cached]=useState(()=>peekPractice<ProgressData>(userId,'progress'));
 const [data,setData]=useState<ProgressData|null>(cached??null),[error,setError]=useState('');
 useEffect(()=>{loadPractice(userId,'progress',()=>quizApi<ProgressData>('/api/practice',{method:'POST',body:JSON.stringify({action:'progress'})})).then(setData).catch(()=>{if(!cached)setError('Progress unavailable; retry by reloading.');});},[userId,cached]);
 return <main className="practice-page"><div className="practice-inner practice-inner--wide">
 <PageHeader eyebrow="Progress" title="Practice progress & retention observations" description="These are practice observations, not evidence that SELAR improves retention. Source matching and AI scores do not establish mastery. Only current source snapshots are included."/>
 {error?<p role="alert" className="practice-error">{error}</p>:!data?<PracticeSkeleton label="Loading observations…" cards={3}/>:<>
 <section className="ui-card practice-section"><h2>Observed delayed recall</h2>
 {data.delayed_unassisted===0?<p>No delayed recall observations yet. Complete a due review at least 24 hours after your previous attempt.</p>:<p>{data.delayed_unassisted} delayed attempts with no reported source or hint use; {data.delayed_scored} AI-scored.</p>}
 {data.delayed_mean_score!==null&&<p>Mean AI practice score: {(data.delayed_mean_score*100).toFixed(0)}% across {data.delayed_scored} scored delayed attempts — not a retention probability.</p>}
 <small>Assistance is self-reported; outside reading cannot be measured. Warm-ups and immediate reading checks are excluded.</small></section>
 <section className="ui-card practice-section"><h2>Practice activity</h2><dl className="practice-stats"><dt>Warm-up attempts</dt><dd>{data.warmup}</dd><dt>Immediate reading checks</dt><dd>{data.reading_check}</dd><dt>Review attempts</dt><dd>{data.review}</dd><dt>Source-exposed / hinted attempts (overlapping count)</dt><dd>{data.exposed}</dd><dt>Provisional / unscored attempts (overlapping count)</dt><dd>{data.unscored}</dd></dl></section>
 <section className="ui-card practice-section"><h2>Schedule and consistency</h2><p>{data.due} items due · {data.streak}-day review streak (UTC calendar days)</p><p>A streak measures participation, not memory.</p><Link className="practice-link" href="/review">Start daily review</Link></section>
 </>}
 <section className="ui-card practice-section"><h2>Estimated memory</h2><p>Not estimated. No calibrated recall model is available. Review dates use a conservative doubling-interval heuristic, not FSRS or an efficacy estimate.</p></section>
 <section className="ui-card practice-section"><h2>Formal study results</h2><p>Fixed admin-authored study assessments remain separate. Generated practice does not alter their items, keys, grading or feedback policies.</p><Link className="practice-link" href="/quizzes">View assigned assessments</Link></section>
 </div></main>;
}
