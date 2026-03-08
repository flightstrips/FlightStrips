import { Link } from "react-router";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { DashedLine } from "./DashedLine";

const categories = [
  {
    title: "Category one",
    questions: [
      { question: "FAQ question placeholder 1?", answer: "FAQ answer placeholder. Replace with real content." },
      { question: "FAQ question placeholder 2?", answer: "FAQ answer placeholder. Replace with real content." },
    ],
  },
  {
    title: "Category two",
    questions: [
      { question: "FAQ question placeholder 3?", answer: "FAQ answer placeholder. Replace with real content." },
      { question: "FAQ question placeholder 4?", answer: "FAQ answer placeholder. Replace with real content." },
    ],
  },
  {
    title: "Category three",
    questions: [
      { question: "FAQ question placeholder 5?", answer: "FAQ answer placeholder. Replace with real content." },
    ],
  },
];

export function Faq() {
  return (
    <section className="py-24 px-6 sm:px-8 bg-white">
      <div className="max-w-3xl mx-auto">
        <div className="flex items-center gap-4 mb-8">
          <DashedLine className="flex-1 border-navy/20" />
          <span className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary whitespace-nowrap">
            FAQ
          </span>
          <DashedLine className="flex-1 border-navy/20" />
        </div>
        <h2
          className="font-display font-semibold text-3xl sm:text-4xl text-navy tracking-tight mb-4"
          style={{ letterSpacing: "-0.02em" }}
        >
          Got questions?
        </h2>
        <p className="text-navy/80 font-light mb-12">
          Can&apos;t find what you&apos;re looking for?{" "}
          <Link to="/about" className="text-primary hover:underline">
            Learn more about us
          </Link>
          , check the{" "}
          <Link to="/faq" className="text-primary hover:underline">
            FAQ
          </Link>
          , or reach out to your vACC.
        </p>

        <Accordion type="single" collapsible className="w-full">
          {categories.map((category) => (
            <div key={category.title} className="mb-8">
              <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-navy/60 mb-4">
                {category.title}
              </p>
              {category.questions.map((item, i) => (
                <AccordionItem key={i} value={`${category.title}-${i}`}>
                  <AccordionTrigger>{item.question}</AccordionTrigger>
                  <AccordionContent>{item.answer}</AccordionContent>
                </AccordionItem>
              ))}
            </div>
          ))}
        </Accordion>
      </div>
    </section>
  );
}
