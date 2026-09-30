import { Link } from "react-router";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";

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
    <section>
      <div className="fsl-gutter py-16 sm:py-24">
        <div className="max-w-3xl">
        <p className="mb-12 text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]">
          Can&apos;t find what you&apos;re looking for? Reach out to your vACC, or{" "}
          <Link to="/contact" className="text-[var(--fsl-brand-ink)] underline underline-offset-4">
            get in touch
          </Link>
          .
        </p>

        <Accordion type="single" collapsible className="w-full">
          {categories.map((category) => (
            <div key={category.title} className="mb-8">
              <p className="fsl-eyebrow mb-4">
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
      </div>
    </section>
  );
}
