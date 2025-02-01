import * as React from "react"

import { Card, CardContent } from "@/components/ui/card"
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from "@/components/ui/carousel"
import Image from "next/image"

export function PatnerCarousel() {
  return (
    <Carousel
      opts={{
        align: "start",
      }}
      className="w-full max-w-xl"
    >
      <CarouselContent>
          <CarouselItem className="basis-1/3 w-full">
            <div className="p-1">
              <Card>
                <CardContent className="flex aspect-video items-center justify-center p-6 bg-[#003d48] rounded-lg">

                </CardContent>
              </Card>
            </div>
          </CarouselItem>
          <CarouselItem className="md:basis-1/2 lg:basis-1/3 w-full">
            <div className="p-1">
              <Card>
                <CardContent className="flex aspect-video items-center justify-center p-6 bg-[#003d48] rounded-lg">
                    <Image src="/White.svg" width="265" height="64" alt="VATSIM Scandinavia"/>
                </CardContent>
              </Card>
            </div>
          </CarouselItem>
          <CarouselItem className="md:basis-1/2 lg:basis-1/3 w-full">
            <div className="p-1">
              <Card>
                <CardContent className="flex aspect-video items-center justify-center p-6 bg-[#003d48] rounded-lg">

                </CardContent>
              </Card>
            </div>
          </CarouselItem>
      </CarouselContent>
      <CarouselPrevious />
      <CarouselNext />
    </Carousel>
  )
}
