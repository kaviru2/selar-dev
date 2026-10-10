"use client";
import {useEffect,useState} from 'react';
import Link from 'next/link';
import {PracticeCard,practiceRPC,type PracticeItem} from '@/components/ReadingPractice';
import {buttonClass} from '@/components/ui/Button';
import {PageHeader} from '@/components/ui/Card';
import {PracticeSkeleton} from '@/components/PracticeSkeleton';
import {useSelarUserId} from '@/lib/context';
import {loadPractice,peekPractice} from '@/lib/practice-cache';
interface Daily {items:PracticeItem[];streak:number}
const fetchDaily=()=>practiceRPC({action:'daily'}) as unknown as Promise<Daily>;
export default function DailyReview(){
 const userId=useSelarUserId();
 // Show this account's last queue at once (if any) and refresh it in the background.
 const [cached]=useState(()=>peekPractice<Daily>(userId,'daily'));
 const [items,setItems]=useState<PracticeItem[]>(cached?.items??[]);const [streak,setStreak]=useState(cached?.streak??0);const [loading,setLoading]=useState(!cached);const [error,setError]=useState('');const [done,setDone]=useState<string[]>([]);
 // State is only set in promise callbacks, never synchronously inside the effect.
 function load(){return loadPractice(userId,'daily',fetchDaily).then(data=>{setError('');setItems(data.items);setStreak(data.streak);},()=>{if(!cached)setError('Daily review unavailable; retry.');}).finally(()=>setLoading(false));}
 useEffect(()=>{void load();},[]); // eslint-disable-line react-hooks/exhaustive-deps
 const remaining=items.filter(i=>!done.includes(i.id));
 return <main className="practice-page"><div className="practice-inner">
 <PageHeader eyebrow="Review" title="Daily review" description={<>{streak}-day review streak · UTC calendar days</>}/><p className="practice-lede">Recall before opening your documents. Mark source/hint use honestly. Schedule: conservative doubling intervals, not a calibrated memory model.</p>
 {loading?<PracticeSkeleton label="Loading due practice…"/>:error?<p role="alert" className="practice-error">{error} <button type="button" className={buttonClass({size:'sm'})} onClick={()=>{setLoading(true);void load();}}>Retry</button></p>:<>
 {!remaining.length&&<p role="status" className="practice-status">Nothing due. {done.length?'Session complete.':'Finish a reading check to schedule tomorrow’s practice.'}</p>}
 {remaining.slice(0,1).map(item=><PracticeCard key={item.id} item={item} phase="review" onDone={()=>{void loadPractice(userId,'daily',fetchDaily).then(data=>setStreak(data.streak)).catch(()=>undefined);}}/>)}
 {remaining.length>0&&<div className="practice-actions"><button type="button" className={buttonClass({size:'sm'})} onClick={()=>setDone([...done,remaining[0].id])}>Next question</button></div>}
 <p className="practice-status">{done.length} of {items.length} questions visited this session</p></>}
 <p className="practice-links"><Link className="practice-link" href="/reader">Read documents</Link> · <Link className="practice-link" href="/progress">Practice progress</Link></p>
 </div></main>;
}
